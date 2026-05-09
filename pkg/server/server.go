package server

import (
	"fmt"
	"net/http"
)

func Run(port int, handler http.Handler) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), handler)
}
