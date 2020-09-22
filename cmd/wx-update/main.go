/*
* Copyright (C) 2020 Nick Naumenko (https://gitlab.com/nnaumenko)
* All rights reserved.
* This software may be modified and distributed under the terms
* of the MIT license. See the LICENSE file for details.
 */

package main

import (
	"log"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nnaumenko/wx/internal/database"
	"github.com/nnaumenko/wx/internal/util"
	"github.com/nnaumenko/wx/internal/wxupdate"
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

	context := wxupdate.UpdateContext{
		Db:                database,
		MetarsLastUpdated: time.Unix(0, 0),
		TafsLastUpdated:   time.Unix(0, 0),
		//		Log: *logger,
	}

	util.Schedule(
		func() {
			wxupdate.GetFromOurAirports(&context)
		}, 24*time.Hour)

	util.Schedule(
		func() {
			wxupdate.UpdateMetars(&context)
		}, 1*time.Minute)

	util.Schedule(
		func() {
			wxupdate.UpdateTafs(&context)
		}, 1*time.Minute)
	for {
		select {}
	}
}
