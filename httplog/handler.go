package httplog

import (
	"log"
	"net/http"
)

// logHandler logs HTTP requests and passes them to the next handler.
func LogHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}
