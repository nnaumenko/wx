/*
* Copyright (C) 2020 Nick Naumenko (https://gitlab.com/nnaumenko)
* All rights reserved.
* This software may be modified and distributed under the terms
* of the MIT license. See the LICENSE file for details.
 */

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/nnaumenko/wx/internal/database"
	"github.com/nnaumenko/wx/internal/wxserver"
)

const (
	addr               = ":9990"
	serverReadTimeout  = 15 * time.Second
	serverWriteTimeout = 15 * time.Second
	serverIdleTimeout  = 15 * time.Second

	enableProfiling             = false
	serverWriteTimeoutProfiling = 180 * time.Second
)

const (
	redisServer = ":6379"

	redisMaxIdleConnections   = 50    // Max idle Redis connections in the pool
	redisMaxActiveConnections = 10000 // Max active Redis connections in the pool
)

func main() {
	pool := redis.Pool{
		MaxIdle:   redisMaxIdleConnections,
		MaxActive: redisMaxActiveConnections,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", redisServer)
			if err != nil {
				log.Fatalf("Unable to create Redis connection pool: %s", err.Error())
			}
			return c, err
		},
	}
	database := database.NewDbAccessRedis(&pool)

	//	logger := log.New(os.Stdout, "wx: ", log.LstdFlags)

	ctx := wxserver.HandlerContext{
		Db: database,
		//		Log: *logger,
	}

	mux := http.NewServeMux()
	wxserver.SetupHandlers(mux, &ctx)

	if enableProfiling {
		mux.Handle("/debug/", http.DefaultServeMux)
	}

	wrTimeout := serverWriteTimeout
	if enableProfiling {
		wrTimeout = serverWriteTimeoutProfiling
	}

	server := http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: wrTimeout,
		IdleTimeout:  serverIdleTimeout,
	}

	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)

	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		log.Println("Shutting down")

		ctxb, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctxb); err != nil {
			log.Fatalf("Unable to shut down: %v\n", err)
		}
		close(done)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Unable to start server: %s\n", err.Error())
	}
	<-done
	log.Println("Server shutdown")
}
