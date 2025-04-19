package main

import (
    "log"
    "net/http"
)

func main() {
    log.Println("Starting service.....")
    http.ListenAndServe(":8080", nil)
}
