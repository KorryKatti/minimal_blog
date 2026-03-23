// following this https://www.digitalocean.com/community/tutorials/how-to-make-an-http-server-in-go

package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
//	"os"
	"context"
	"net"
)

const keyServerAddr = "serverAddr"

func main() {
	fmt.Println("Hello, minimal blog!")

	mux := http.NewServeMux()
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/hello", getHello)

	ctx, cancelCtx := context.WithCancel(context.Background())

	serverOne := &http.Server{
		Addr:    ":3456",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			return context.WithValue(ctx, keyServerAddr, l.Addr().String())
		},
	}

	serverTwo := &http.Server{
		Addr:    ":4444",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			return context.WithValue(ctx, keyServerAddr, l.Addr().String())
		},
	}

	go func() {
		err := serverOne.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			fmt.Println("server one closed")
		} else if err != nil {
			fmt.Printf("error listening for server one: %s\n", err)
		}
		cancelCtx()
	}()

	go func() {
		err := serverTwo.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			fmt.Println("server two closed")
		} else if err != nil {
			fmt.Printf("error listening for server two: %s\n", err)
		}
		cancelCtx()
	}()

	<-ctx.Done()

	fmt.Println("shutting down servers...")
}
func getRoot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fmt.Printf("%s: got / request\n",ctx.Value(keyServerAddr))
	io.WriteString(w, "this is my website\n")
}

func getHello(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fmt.Printf("%s got /hello request\n",ctx.Value(keyServerAddr))
	io.WriteString(w, " Hello, HTTP!\n")
}

