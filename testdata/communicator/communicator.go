package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	target := os.Getenv("CM_TARGET")

	var status string

	mux := http.DefaultServeMux

	// get `status` value
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(status))
	})

	// set `status` value and return old `status`
	mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		text, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		old := status
		status = string(text)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(old))
	})

	// send `status` value to target, and receives target's `status`
	mux.HandleFunc("/exchange", func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Post(
			fmt.Sprintf("http://%s/set", target),
			"plain/text",
			strings.NewReader(status),
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if resp.StatusCode != http.StatusOK {
			http.Error(w, http.StatusText(resp.StatusCode), http.StatusInternalServerError)
			return
		}

		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		status = string(data)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	err := http.ListenAndServe(":80", mux)
	if err != nil {
		log.Fatal(err)
	}
}
