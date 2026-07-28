package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	loadTemplates()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/simple_index", simpleIndexHandler)
	http.HandleFunc("/simple_post", simplePostHandler)
	http.HandleFunc("/sync_index", syncIndexHandler)
	http.HandleFunc("/sync_post", syncPostHandler)
	http.HandleFunc("/sync_confirm", syncConfirmHandler)
	http.HandleFunc("/async_index", asyncIndexHandler)
	http.HandleFunc("/async_post", asyncPostHandler)
	http.HandleFunc("/async_oauth_response", asyncOAuthResponseHandler)
	http.HandleFunc("/async_confirm", asyncConfirmHandler)
	http.HandleFunc("/sig", sigHandler)
	http.HandleFunc("/sig_generate", sigGenerateHandler)
	http.HandleFunc("/sig_fetch", sigFetchHandler)
	http.HandleFunc("/sig_verify", sigVerifyHandler)
	http.HandleFunc("/sig_verify_url", sigVerifyURLHandler)
	http.HandleFunc("/client", clientHandler)
	http.HandleFunc("/ddnscode", ddnsCodeHandler)
	http.HandleFunc("/static/", staticHandler)

	port := "8080"
	fmt.Printf("Starting server on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
