/*
* Copyright (C) 2020 Nick Naumenko (https://gitlab.com/nnaumenko)
* All rights reserved.
* This software may be modified and distributed under the terms
* of the MIT license. See the LICENSE file for details.
 */

package database

import (
	"fmt"
	"strconv"

	"github.com/gomodule/redigo/redis"
)

// DataICAOLocation is the data retreived from database for a single location
// designated by an ICAO location code.
// Has JSON tags to be marshalled easily.
type DataICAOLocation struct {
	Location       string  `json:"location,omitempty"`
	Metar          string  `json:"metar,omitempty"`
	Taf            string  `json:"taf,omitempty"`
	Name           string  `json:"name,omitempty"`
	City           string  `json:"city,omitempty"`
	CountryCode    string  `json:"country_code,omitempty"`
	Latitude       float64 `json:"latitude,omitempty"`
	Longitude      float64 `json:"longitude,omitempty"`
	AltitudeMeters int     `json:"altitude_meters,omitempty"`
	AltitudeFeet   int     `json:"altitude_feet,omitempty"`
}

// Database interface is an abstraction for database which stores the weather
// data
type Database interface {
	// GetDataICAO retreives all available data for one or more ICAO
	// locations, including location data and active METAR & TAF.
	// Does not validate ICAO locations passed in loc argument.
	// Does not limit number of locations.
	// Locations not found in the database are not included in the slice.
	// All fields of DataICAOLocation are intialised.
	GetICAOLocationData(loc []string) ([]*DataICAOLocation, error)

	// GetMETARs retreives only METAR reports for one or more ICAO locations.
	// Does not validate ICAO locations passed in loc argument.
	// Does not limit number of locations.
	// Locations not found in the database are not included in the slice.
	// Only Location and Metar fields are initialised in DataICAOLocation.
	GetMETARs(loc []string) ([]*DataICAOLocation, error)

	// GetTAFs retreives only METAR reports for one or more ICAO locations.
	// Does not validate ICAO locations passed in loc argument.
	// Does not limit number of locations.
	// Locations not found in the database are not included in the slice.
	// Only Location and Taf fields are initialised in DataICAOLocation.
	GetTAFs(loc []string) ([]*DataICAOLocation, error)

	// GetMETARsTAFs retreives only METAR and TAF reports for ICAO locations.
	// Does not validate ICAO locations passed in loc argument.
	// Does not limit number of locations.
	// Locations not found in the database are not included in the slice.
	// Only Location, Metar and Taf fields are initialised in DataICAOLocation.
	GetMETARsTAFs(loc []string) ([]*DataICAOLocation, error)

	// LocationExists checks whether an ICAO location exists in the database.
	// Does not validate ICAO location.
	LocationExists(loc string) (bool, error)

	// SetDataICAOLocation sets the location data in the database.
	// Only Location, Name, City, CountryCode, Latitude, Longitude,
	// AltitudeFeet fields are saved from DataICAOLocation to database.
	SetDataICAOLocation(data *DataICAOLocation) error

	// SetMETAR sets or updates single METAR for an ICAO location.
	// Does not validate ICAO location.
	// Expire is the time-to-expire for the METAR in seconds.
	SetMETAR(loc string, metar string, expire int64) error

	// SetTAF sets or updates single TAF for an ICAO location.
	// Does not validate ICAO location.
	// Expire is the time-to-expire for the METAR in seconds.
	SetTAF(loc string, taf string, expire int64) error
}

////////////////////////////////////////////////////////////////////////////////

// DbRedis is an implementation of retreival data stored in Redis
type DbRedis struct {
	pool *redis.Pool
}

const (
	DbRedisICAOPrefixLocation = "wx:icao:loc:"
	DbRedisICAOPrefixMetar    = "wx:icao:metar:"
	DbRedisICAOPrefixTaf      = "wx:icao:taf:"

	DbRedisICAOLocFieldName         = "name"
	DbRedisICAOLocFieldCity         = "city"
	DbRedisICAOLocFieldCountryCode  = "country"
	DbRedisICAOLocFieldLatitude     = "lat"
	DbRedisICAOLocFieldLongitude    = "lon"
	DbRedisICAOLocFieldAltitudeFeet = "alt_ft"
)

// GetICAOLocationData retreives selected data fields for ICAO locations.
// See Database interface for details.
func (db *DbRedis) GetICAOLocationData(loc []string) ([]*DataICAOLocation, error) {
	metars, err := db.getMetarStrs(loc)
	if err != nil {
		return make([]*DataICAOLocation, 0), err
	}
	tafs, err := db.getTafStrs(loc)
	if err != nil {
		return make([]*DataICAOLocation, 0), err
	}

	var result []*DataICAOLocation
	conn := db.pool.Get()
	defer conn.Close()

	for i, l := range loc {
		v, err := redis.StringMap(conn.Do("HGETALL", DbRedisICAOPrefixLocation+l))
		if err != nil {
			return make([]*DataICAOLocation, 0), err
		}
		if len(v) > 0 {
			ld, err := db.makeLocationData(l, v)
			if err != nil {
				return make([]*DataICAOLocation, 0), err
			}
			ld.Metar = metars[i]
			ld.Taf = tafs[i]
			result = append(result, ld)
		}
	}
	return result, nil
}

// GetMETARs retreives only METAR reports for ICAO locations.
// See Database interface for details.
func (db *DbRedis) GetMETARs(loc []string) ([]*DataICAOLocation, error) {
	var result []*DataICAOLocation
	metars, err := db.getMetarStrs(loc)
	if err != nil {
		return make([]*DataICAOLocation, 0), err
	}
	for i, metar := range metars {
		if len(metar) > 0 {
			var l DataICAOLocation
			l.Location = loc[i]
			l.Metar = metar
			result = append(result, &l)
		}
	}
	return result, nil
}

// GetTAFs retreives only TAF reports for ICAO locations.
// See Database interface for details.
func (db *DbRedis) GetTAFs(loc []string) ([]*DataICAOLocation, error) {
	var result []*DataICAOLocation
	tafs, err := db.getTafStrs(loc)
	if err != nil {
		return make([]*DataICAOLocation, 0), err
	}
	for i, metar := range tafs {
		if len(metar) > 0 {
			var l DataICAOLocation
			l.Location = loc[i]
			l.Taf = metar
			result = append(result, &l)
		}
	}
	return result, nil
}

// GetMETARsTAFs retreives only METAR and TAF reports for ICAO locations.
// See Database interface for details.
func (db *DbRedis) GetMETARsTAFs(loc []string) ([]*DataICAOLocation, error) {
	var result []*DataICAOLocation
	conn := db.pool.Get()
	defer conn.Close()

	m, err := db.getMetarStrs(loc)
	if err != nil {
		return make([]*DataICAOLocation, 0), err
	}
	t, err := db.getTafStrs(loc)
	if err != nil {
		return make([]*DataICAOLocation, 0), err
	}
	if len(m) != len(t) {
		panic("GetMETARsTAFs: METARs vs TAFs length mismatch")
	}
	for i := 0; i < len(m); i++ {
		if len(m) > 0 {
			var l DataICAOLocation
			l.Location = loc[i]
			l.Metar = m[i]
			l.Taf = t[i]
			result = append(result, &l)
		}
	}
	return result, nil
}

// LocationExists checks whether an ICAO location exists in the database.
// See Database interface for details.
func (db *DbRedis) LocationExists(loc string) (bool, error) {
	conn := db.pool.Get()
	defer conn.Close()
	result, err := redis.Bool(conn.Do("EXISTS", DbRedisICAOPrefixLocation+loc))
	return result, err
}

// SetDataICAOLocation sets the location data in the database.
// See Database interface for details.
func (db *DbRedis) SetDataICAOLocation(data *DataICAOLocation) error {
	conn := db.pool.Get()
	defer conn.Close()
	exists, err := redis.Bool(conn.Do("EXISTS", DbRedisICAOPrefixLocation+data.Location))
	if err != nil {
		return fmt.Errorf("EXISTS command returned error: %s", err.Error())
	}
	if !exists {
		_, err := conn.Do("HSET",
			DbRedisICAOPrefixLocation+data.Location,
			DbRedisICAOLocFieldName, data.Name,
			DbRedisICAOLocFieldCity, data.City,
			DbRedisICAOLocFieldCountryCode, data.CountryCode,
			DbRedisICAOLocFieldLatitude, data.Latitude,
			DbRedisICAOLocFieldLongitude, data.Longitude,
			DbRedisICAOLocFieldAltitudeFeet, data.AltitudeFeet,
		)
		return err
	}
	return nil
}

// SetMETAR sets or updates single METAR for a location
// See Database interface for details.
func (db *DbRedis) SetMETAR(loc string, metar string, expire int64) error {
	conn := db.pool.Get()
	defer conn.Close()
	_, err := conn.Do("SET", DbRedisICAOPrefixMetar+loc, metar, "EX", expire)
	return err
}

// SetTAF sets or updates single TAF for a location
// See Database interface for details.
func (db *DbRedis) SetTAF(loc string, taf string, expire int64) error {
	conn := db.pool.Get()
	defer conn.Close()
	_, err := conn.Do("SET", DbRedisICAOPrefixTaf+loc, taf, "EX", expire)
	return err
}

func (db *DbRedis) makeLocationData(loc string, s map[string]string) (*DataICAOLocation, error) {
	var l DataICAOLocation
	alt, err := strconv.Atoi(s[DbRedisICAOLocFieldAltitudeFeet])
	if err != nil {
		return &l, err
	}
	lat, err := strconv.ParseFloat(s[DbRedisICAOLocFieldLatitude], 64)
	if err != nil {
		return &l, err
	}
	lon, err := strconv.ParseFloat(s[DbRedisICAOLocFieldLongitude], 64)
	if err != nil {
		return &l, err
	}
	l.Location = loc
	l.Name = s[DbRedisICAOLocFieldName]
	l.City = s[DbRedisICAOLocFieldCity]
	l.CountryCode = s[DbRedisICAOLocFieldCountryCode]
	l.AltitudeFeet = alt
	l.AltitudeMeters = int(alt * 3048 / 10000)
	l.Latitude = lat
	l.Longitude = lon
	return &l, nil
}

func (db *DbRedis) getMetarStrs(loc []string) ([]string, error) {
	conn := db.pool.Get()
	defer conn.Close()
	var li []interface{}
	for _, l := range loc {
		li = append(li, DbRedisICAOPrefixMetar+l)
	}
	return redis.Strings(conn.Do("MGET", li...))
}

func (db *DbRedis) getTafStrs(loc []string) ([]string, error) {
	conn := db.pool.Get()
	defer conn.Close()
	var li []interface{}
	for _, l := range loc {
		li = append(li, DbRedisICAOPrefixTaf+l)
	}
	return redis.Strings(conn.Do("MGET", li...))
}

// NewDbAccessRedis is a factory function to create an instance of
// DbRedis. ConnectionPool redis.Pool must be initialised by others than
// NewDbAccessRedis.
func NewDbAccessRedis(p *redis.Pool) Database {
	db := DbRedis{pool: p}
	return &db
}
