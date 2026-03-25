// following this https://www.digitalocean.com/community/tutorials/how-to-make-an-http-server-in-go

package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"io/ioutil"
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

	server := &http.Server{
		Addr:":3333",
		Handler:mux,
		BaseContext: func(l net.Listener)context.Context {
			ctx = context.WithValue(ctx,keyServerAddr,l.Addr().String())
			return ctx
		},
	}

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error listening for server: %s\n", err)
	}
	

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
	hasFirst := r.URL.Query().Has("first")
	first := r.URL.Query().Get("first")
	hasSecond :=  r.URL.Query().Has("second")
	second := r.URL.Query().Get("second")

	body,err := ioutil.ReadAll(r.Body)
	if err != nil{
		fmt.Printf("couldn't read body %s\n",err)
	}

	fmt.Println("%s: got /request first(%t)=%s, second(%t)=%s, body:\n%s\n",ctx.Value(keyServerAddr),hasFirst,first,hasSecond,second,string(body))

	io.WriteString(w, "this is my website\n")
}

func getHello(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fmt.Printf("%s got /hello request\n",ctx.Value(keyServerAddr))

	myName := r.PostFormValue("myName")
	if myName == ""{
		w.Header().Set("x-missing-field","myName")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	io.WriteString(w,fmt.Sprintf("Hello, %s\n", myName))
}
