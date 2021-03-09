package session

import (
	"bytes"
	"fmt"
	"github.com/go-redis/redis/v8"
	"net/http"
	"testing"
)

// ResponseRecorder is an implementation of http.ResponseWriter that
// records its mutations for later inspection in tests.
type ResponseRecorder struct {
	Code      int           // the HTTP response code from WriteHeader
	HeaderMap http.Header   // the HTTP response headers
	Body      *bytes.Buffer // if non-nil, the bytes.Buffer to append written data to
	Flushed   bool
}

// NewRecorder returns an initialized ResponseRecorder.
func NewRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		HeaderMap: make(http.Header),
		Body:      new(bytes.Buffer),
	}
}

// Header returns the response headers.
func (rw *ResponseRecorder) Header() http.Header {
	return rw.HeaderMap
}

// Write always succeeds and writes to rw.Body, if not nil.
func (rw *ResponseRecorder) Write(buf []byte) (int, error) {
	if rw.Body != nil {
		rw.Body.Write(buf)
	}
	if rw.Code == 0 {
		rw.Code = http.StatusOK
	}
	return len(buf), nil
}

// WriteHeader sets rw.Code.
func (rw *ResponseRecorder) WriteHeader(code int) {
	rw.Code = code
}

// Flush sets rw.Flushed to true.
func (rw *ResponseRecorder) Flush() {
	rw.Flushed = true
}

func TestNewRedisStoreWithDB(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       8,
		PoolSize: 10,
	})
	db, err := NewRedisStoreWithDB(client, []byte("new-key"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	req, _ := http.NewRequest("GET", "http://localhost:8888/", nil)
	rsp := NewRecorder()
	ss, err := db.Get(req, "session-key")
	if err != nil {
		t.Fatalf("Error getting session :%v", err)
	}
	flashes := ss.Flashes()
	if len(flashes) != 0 {
		t.Errorf("Expected empty flashes; Got %v\n", flashes)
	}
	ss.AddFlash("hello")
	ss.AddFlash("world")
	ss.AddFlash("pong", "ping")
	err = ss.Save(req, rsp)

	if err != nil {
		t.Fatalf("Error Saving session: %v\n", err)
	}
	hdr := rsp.Header()
	cookies, ok := hdr["Set-Cookie"]
	if !ok || len(cookies) != 1 {
		t.Fatalf("No cookies. Header: %v\n", hdr)
	}
	fmt.Printf("Heder: %v\n", hdr)
	fmt.Printf("Flashed: %v\n", ss.Flashes("ping"))
}
