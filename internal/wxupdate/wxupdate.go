/*
* Copyright (C) 2020 Nick Naumenko (https://gitlab.com/nnaumenko)
* All rights reserved.
* This software may be modified and distributed under the terms
* of the MIT license. See the LICENSE file for details.
 */

package wxupdate

import (
	"encoding/csv"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/nnaumenko/wx/internal/database"
	"github.com/nnaumenko/wx/internal/util"
)

const (
	avcMetarURL string = "https://www.aviationweather.gov/adds/dataserver_current/current/metars.cache.csv"
	avcTafURL   string = "https://www.aviationweather.gov/adds/dataserver_current/current/tafs.cache.csv"

	avcMetarCsvFieldRawText         string = "raw_text"
	avcMetarCsvFieldStationID       string = "station_id"
	avcMetarCsvFieldObservationTime string = "observation_time"
	avcMetarCsvFieldMetarType       string = "metar_type"

	avcTafCsvFieldRawText     string = "raw_text"
	avcTafCsvFieldStationID   string = "station_id"
	avcTafCsvFieldValidTimeTo string = "valid_time_to"
)

const (
	ourairportsAirportsCsv  string = "https://ourairports.com/data/airports.csv"
	ourairportsCountriesCsv string = "https://ourairports.com/data/countries.csv"
	ourairportsRegionsCsv   string = "https://ourairports.com/data/regions.csv"

	ourairportsAirportsCsvFieldType         string = "type"
	ourairportsAirportsCsvFieldName         string = "name"
	ourairportsAirportsCsvFieldLatitudeDeg  string = "latitude_deg"
	ourairportsAirportsCsvFieldLongitudeDeg string = "longitude_deg"
	ourairportsAirportsCsvFieldElevationFt  string = "elevation_ft"
	ourairportsAirportsCsvFieldIsoCountry   string = "iso_country"
	ourairportsAirportsCsvFieldIsoRegion    string = "iso_region"
	ourairportsAirportsCsvFieldMunicipality string = "municipality"
	ourairportsAirportsCsvFieldGpsCode      string = "gps_code"
)

// UpdateContext is passed to endpoint handlers
type UpdateContext struct {
	Db                database.Database
	MetarsLastUpdated time.Time
	TafsLastUpdated   time.Time
	Log               log.Logger
}

// UpdateMetars retreives METAR data from aviationweather.gov
func UpdateMetars(ctx *UpdateContext) {
	log.Println("Updating METARs")
	start := time.Now()
	metars, err := util.GetFromURL(avcMetarURL, ctx.MetarsLastUpdated)
	if err != nil {
		log.Printf("Error retreiving %s: %s", avcMetarURL, err.Error())
		return
	}
	if metars == nil {
		log.Printf("METARs not updated since last update")
		return
	}
	defer metars.Close()
	ctx.MetarsLastUpdated = time.Now()
	log.Printf("Downloaded METARs in %v", time.Now().Sub(start))

	start, num := time.Now(), 0
	r := csv.NewReader(metars)
	fieldNames := []string{
		avcMetarCsvFieldRawText,
		avcMetarCsvFieldStationID,
		avcMetarCsvFieldObservationTime,
		avcMetarCsvFieldMetarType}
	fieldIdx, err := util.ParseCsvHeader(r, fieldNames)
	if err != nil {
		log.Printf("Error parsing header of METARs CSV %s", err.Error())
		return
	}
	for i, idx := range fieldIdx {
		if idx < 0 {
			log.Printf("Field %s not found in METAR CSV", fieldNames[i])
			return
		}
	}
	colRawText := fieldIdx[0]
	colStation := fieldIdx[1]
	colObsTime := fieldIdx[2]
	colType := fieldIdx[3]

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading METAR CSV: %s : %v", err.Error(), record)
			return
		}
		expire, err := util.ExpireSeconds(record[colObsTime], 3600*3)
		if err != nil {
			log.Printf("Cannot parse METAR time %s: %s",
				record[colObsTime], err.Error())
		}
		metar := record[colType] + " " + record[colRawText]
		err = ctx.Db.SetMETAR(record[colStation], metar, expire)
		if err != nil {
			log.Printf("Cannot update METAR %s (expires in %d sec): %s",
				metar, expire, err.Error())
		}
		num++
	}
	log.Printf("Updated %d METARs in %v", num, time.Now().Sub(start))
}

// UpdateTafs retreives TAF data from avaitionweather.gov
func UpdateTafs(ctx *UpdateContext) {
	log.Println("Updating TAFs")
	start := time.Now()
	tafs, err := util.GetFromURL(avcTafURL, ctx.TafsLastUpdated)
	if err != nil {
		log.Printf("Error retreiving TAFs %s: %s", avcTafURL, err.Error())
		return
	}
	if tafs == nil {
		log.Printf("TAFs not updated since last update")
		return
	}
	defer tafs.Close()
	ctx.TafsLastUpdated = time.Now()
	log.Printf("Downloaded TAFs in %v", time.Now().Sub(start))

	start, num := time.Now(), 0
	r := csv.NewReader(tafs)
	fieldNames := []string{
		avcTafCsvFieldRawText,
		avcTafCsvFieldStationID,
		avcTafCsvFieldValidTimeTo}
	fieldIdx, err := util.ParseCsvHeader(r, fieldNames)
	if err != nil {
		log.Printf("Error parsing header of TAFs CSV %s", err.Error())
		return
	}
	for i, idx := range fieldIdx {
		if idx < 0 {
			log.Printf("Field %s not found in TAFs CSV", fieldNames[i])
			return
		}
	}
	colRawText, colStation, colTimeTo := fieldIdx[0], fieldIdx[1], fieldIdx[2]
	r.FieldsPerRecord = -1

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading TAFs CSV: %s : %v", err.Error(), record)
			return
		}
		expire, err := util.ExpireSeconds(record[colTimeTo], 0)
		if err != nil {
			log.Printf("Cannot parse TAFs time 'to' %s: %s",
				record[colTimeTo], err.Error())
		}
		err = ctx.Db.SetTAF(record[colStation], record[colRawText], expire)
		if err != nil {
			log.Printf("Cannot update METAR %s (expires in %d sec): %s",
				record[colRawText], expire, err.Error())
		}
		num++
	}
	log.Printf("Updated %d TAFs in %v", num, time.Now().Sub(start))
}

// GetFromOurAirports imports station data for ICAO locations from
// ourairports.com
func GetFromOurAirports(ctx *UpdateContext) {
	log.Println("Importing from OurAirports")
	start := time.Now()
	airports, err := util.GetFromURL(ourairportsAirportsCsv, time.Unix(0, 0))
	if err != nil {
		log.Printf("Error retreiving OurAirports airport database %s: %s", ourairportsAirportsCsv, err.Error())
		return
	}
	if airports == nil {
		log.Printf("OurAirports airport database not updated since last update")
		return
	}
	defer airports.Close()
	log.Printf("Downloaded Airports database in %v", time.Now().Sub(start))
	start, num := time.Now(), 0
	r := csv.NewReader(airports)
	fieldNames := []string{
		ourairportsAirportsCsvFieldType,
		ourairportsAirportsCsvFieldName,
		ourairportsAirportsCsvFieldLatitudeDeg,
		ourairportsAirportsCsvFieldLongitudeDeg,
		ourairportsAirportsCsvFieldElevationFt,
		ourairportsAirportsCsvFieldIsoCountry,
		ourairportsAirportsCsvFieldIsoRegion,
		ourairportsAirportsCsvFieldMunicipality,
		ourairportsAirportsCsvFieldGpsCode}
	fieldIdx, err := util.ParseCsvHeader(r, fieldNames)
	if err != nil {
		log.Printf("Error parsing header of ourairports airport CSV %s", err.Error())
		return
	}
	for i, idx := range fieldIdx {
		if idx < 0 {
			log.Printf("Field %s not found in ourairports airport CSV", fieldNames[i])
			return
		}
	}
	colType, colName := fieldIdx[0], fieldIdx[1]
	colLat, colLon, colAlt := fieldIdx[2], fieldIdx[3], fieldIdx[4]
	colCountryCode /*colRegionCode,*/, colCity := fieldIdx[5] /*fieldIdx[6],*/, fieldIdx[7]
	colICAOCode := fieldIdx[8]

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading ourairports airport CSV: %s : %v", err.Error(), record)
			return
		}

		if record[colType] != "closed" && util.ValidateICAOLocation(record[colICAOCode]) {
			alt, erralt := strconv.Atoi(record[colAlt])
			if erralt != nil {
				log.Printf("Atoi error %s parsing %s in %v", erralt.Error(), record[colICAOCode], record)
			}
			lat, errlat := strconv.ParseFloat(record[colLat], 64)
			if errlat != nil {
				log.Printf("ParseFloat error %s parsing %s in %v", errlat.Error(), record[colLat], record)
			}
			lon, errlon := strconv.ParseFloat(record[colLon], 64)
			if errlon != nil {
				log.Printf("ParseFloat error %s parsing %s in %v", errlon.Error(), record[colLon], record)
			}
			if erralt == nil && errlat == nil && errlon == nil {
				dl := database.DataICAOLocation{
					Location:     record[colICAOCode],
					Name:         record[colName],
					City:         record[colCity],
					CountryCode:  record[colCountryCode],
					Latitude:     lat,
					Longitude:    lon,
					AltitudeFeet: alt,
				}
				err = ctx.Db.SetDataICAOLocation(&dl)
				if err != nil {
					log.Printf("Cannot set ICAO location %v: %s", record, err.Error())
				}
				num++
			}
		}

	}
	log.Printf("Updated %d locations from ourairport database in %v", num, time.Now().Sub(start))
}
