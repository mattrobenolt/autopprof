package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.withmatt.com/autopprof"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	cfg := autopprof.Config{
		Interval:   1 * time.Second,
		Directory:  "./profiles",
		Check:      autopprof.MemoryLimit(1 * 1024 * 1024), // 5MB
		MaxRecords: 3,
		ErrorLog: func(err error) {
			fmt.Println(err)
		},
	}

	go func() {
		if err := cfg.Start(ctx); err != nil {
			panic(err)
		}
	}()

	go func() {
		http.ListenAndServe("127.0.0.1:8000", cfg.Handler())
	}()

	junk := make([]byte, 0)

	go func() {
		for {
			select {
			case <-time.After(10 * time.Millisecond):
				for i := 0; i < 500; i++ {
					junk = append(junk, 0, 0, 0, 0, 0)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()
}
