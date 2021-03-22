package gsr

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
	"strings"
	"time"
)

var (
	defaultMaxAge = 60 * 20
	sessionExpire = 86400 * 30
)

//Serializer
type Serializer interface {
	Deserialize(d []byte, ss *sessions.Session) error
	Serialize(ss *sessions.Session) ([]byte, error)
}

//JSONSerializer 使用json数据格式人为可读
type JSONSerializer struct{}

func (s JSONSerializer) Serialize(ss *sessions.Session) ([]byte, error) {
	m := make(map[string]interface{}, len(ss.Values))
	for k, v := range ss.Values {
		ks, ok := k.(string)
		if !ok {
			err := fmt.Errorf("Non-string key value, cannot serialize session to JSON: %v\n", k)
			return nil, err
		}
		m[ks] = v
	}
	return json.Marshal(m)
}

func (s JSONSerializer) Deserialize(d []byte, ss *sessions.Session) error {
	m := make(map[string]interface{})
	err := json.Unmarshal(d, &m)
	if err != nil {
		return err
	}
	for k, v := range m {
		ss.Values[k] = v
	}
	return nil
}

//GobSerializer go 特有编码方式，此种编码数据不能与其他语言通信
type GobSerializer struct{}

func (s GobSerializer) Serialize(ss *sessions.Session) ([]byte, error) {
	buf := new(bytes.Buffer)
	encode := gob.NewEncoder(buf)
	err := encode.Encode(ss.Values)
	if err == nil {
		return buf.Bytes(), nil
	}
	return nil, err
}

func (s GobSerializer) Deserialize(d []byte, ss *sessions.Session) error {
	decoder := gob.NewDecoder(bytes.NewBuffer(d))
	return decoder.Decode(&ss.Values)
}

type RedisStore struct {
	client        *redis.Client
	Codecs        []securecookie.Codec
	Options       *sessions.Options
	DefaultMaxAge int
	maxLength     int
	keyPrefix     string
	serializer    Serializer
}

func (s *RedisStore) SetMaxLength(l int) {
	if l >= 0 {
		s.maxLength = l
	}
}

func (s *RedisStore) SetKeyPrefix(p string) {
	s.keyPrefix = p
}

func (s *RedisStore) SetSerializer(ss Serializer) {
	s.serializer = ss
}

func (s *RedisStore) SetMaxAge(v int) {
	var c *securecookie.SecureCookie
	var ok bool
	s.Options.MaxAge = v
	for i := range s.Codecs {
		if c, ok = s.Codecs[i].(*securecookie.SecureCookie); ok {
			c.MaxAge(v)
		} else {
			fmt.Printf("Can't change MaxAge on codec %v\n", s.Codecs[i])
		}
	}
}

func NewRedisStoreWithDB(ctx context.Context, client *redis.Client, keyPairs ...[]byte) (*RedisStore, error) {
	rs := &RedisStore{
		client: client,
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: sessionExpire,
		},
		DefaultMaxAge: defaultMaxAge,
		maxLength:     4096,
		keyPrefix:     "gosession_",
		serializer:    GobSerializer{}, // JSONSerializer{} 使用json数据格式人为可读
	}
	return rs, rs.client.Ping(ctx).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

func (s *RedisStore) New(r *http.Request, name string) (*sessions.Session, error) {
	var (
		err error
		ok  bool
	)
	session := sessions.NewSession(s, name)
	options := *s.Options
	session.Options = &options
	session.IsNew = true
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if err == nil {
			ok, err = s.load(r.Context(), session)
			session.IsNew = !(err == nil && ok)
		}
	}
	return session, err
}

func (s *RedisStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	if session.Options.MaxAge <= 0 {
		if err := s.delete(r.Context(), session); err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
	} else {
		if session.ID == "" {
			session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "")
		}
		if err := s.save(r.Context(), session); err != nil {
			return err
		}
		encode, err := securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
		if err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), encode, session.Options))
	}
	return nil
}

func (s *RedisStore) save(ctx context.Context, session *sessions.Session) error {
	b, err := s.serializer.Serialize(session)
	if err != nil {
		return err
	}
	if s.maxLength != 0 && len(b) > s.maxLength {
		return errors.New("SessionStore the value to store is too big, you need set more maxLength")
	}
	age := session.Options.MaxAge
	if age == 0 {
		age = s.DefaultMaxAge
	}
	err = s.client.Set(ctx, s.keyPrefix+session.ID, b, time.Duration(age)*time.Second).Err()
	return err
}

func (s *RedisStore) load(ctx context.Context, session *sessions.Session) (bool, error) {
	b, err := s.client.Get(ctx, s.keyPrefix+session.ID).Bytes()
	switch {
	case err == redis.Nil:
		return false, err
	case err != nil:
		return false, err
	case len(b) == 0:
		return false, err
	default:
		return true, s.serializer.Deserialize(b, session)
	}
}

func (s *RedisStore) delete(ctx context.Context, session *sessions.Session) error {
	return s.client.Del(ctx, s.keyPrefix+session.ID).Err()
}
