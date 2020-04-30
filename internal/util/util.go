/*
* Copyright (C) 2020 Nick Naumenko (https://gitlab.com/nnaumenko)
* All rights reserved.
* This software may be modified and distributed under the terms
* of the MIT license. See the LICENSE file for details.
 */

package util

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
)

// SetCORSHeaders modifies headers of http.ResponseWriter by adding headers
// which allow CORS requests
func SetCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
}

// ServeOptions form a response of an OPTIONS request. If the request is a
// preflight CORS request, corresponding CORS headers are set. If the request
//
func ServeOptions(w http.ResponseWriter, r *http.Request, readOnly bool, allowCORS bool) {
	m := r.Header.Get("Access-Control-Request-Method")
	h := r.Header.Get("Access-Control-Request-Headers")
	o := r.Header.Get("Origin")
	if allowCORS && (len(m) > 0 || len(h) > 0 || len(o) > 0) {
		// Respond to a preflight CORS request
		SetCORSHeaders(w)
	} else {
		// Respond to a query for allowed request methods
		if readOnly {
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		} else {
			w.Header().Set("Allow", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
		}
		w.Header().Set("Cache-control", "no-cache")
	}
	w.WriteHeader(http.StatusNoContent)
}

// ParseURLQueryList parses a list specified in a URL query
// For example list "item1,item2,item3" results in slice {"item1", "item2",
// "item3"}
func ParseURLQueryList(queryValues []string) []string {
	var result []string
	for _, qv := range queryValues {
		elem := strings.Split(qv, ",")
		result = append(result, elem...)
	}
	return result
}

// GetFromURL performs a GET request to specified URL to get the content.
// The returned io.ReadCloser MUST be closed by caller.
func GetFromURL(url string, lastUpdated time.Time) (io.ReadCloser, error) {
	netTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 60 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 60 * time.Second,
	}
	httpClient := &http.Client{
		Timeout:   time.Second * 60,
		Transport: netTransport,
	}
	head, err := httpClient.Head(url)
	if err != nil {
		return nil, fmt.Errorf("HEAD request to %s error: %s", url, err.Error())
	}
	head.Body.Close()
	if head.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HEAD request to %s resulted in code %d", url, head.StatusCode)
	}
	lastModified := head.Header["Last-Modified"]
	if len(lastModified) == 1 {
		lastModTime, err := time.Parse(time.RFC1123, lastModified[0])
		if err != nil {
			return nil, fmt.Errorf("Cannot parse Last-Modified: %s (requested %s)", lastModified[0], url)
		}
		if lastModTime.Before(lastUpdated) {
			return nil, nil
		}
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Request to %s resulted in code %d", url, resp.StatusCode)
	}
	return resp.Body, err
}

// ValidateICAOLocation validates a string for accordance to ICAO location rules.
// The ICAO location pattern is [A-Z]([A-Z0-9]){3}
func ValidateICAOLocation(loc string) bool {
	if len(loc) != 4 {
		return false
	}
	if loc[0] < 'A' && loc[1] > 'Z' {
		return false
	}
	for i := 1; i < len(loc); i++ {
		c := loc[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// ParseCsvHeader skips leading header lines in CSV file.
// Some CSV files may have one or more info/diagnostic lines at the beginning
// of the file followed by line of column names.
// ParseCsvHeader does two things.
// 1. Skips the lines at the beginning of the where the csv delimiter was not
// found.
// 2. After it finds a line with field names, it searches this line for the
// field names specified in fieldNames parameter and returns the slice of
// integer with indecies of specified field names.
// 3. It sets FieldsPerRecord of the csv.Reader to the number of fields found
// in the field names' line
func ParseCsvHeader(src *csv.Reader, fieldNames []string) ([]int, error) {
	if len(fieldNames) < 1 {
		return make([]int, 0), errors.New("No field names specified")
	}
	result := make([]int, len(fieldNames))
	for i := range result {
		result[i] = -1
	}
	src.FieldsPerRecord = -1
	for {
		record, err := src.Read()
		if err != nil {
			return result,
				fmt.Errorf("Error %s parsing CSV header: %v", err.Error(), record)
		}
		if len(record) > 1 {
			src.FieldsPerRecord = len(record)
			for i, s := range record {
				for j, f := range fieldNames {
					if s == f && result[j] == -1 {
						result[j] = i
					}
				}
			}
			break
		}
	}
	return result, nil
}

// ExpireSeconds calculates expiration period in seconds since current moment,
// based on the start date and expiration period since start date.
func ExpireSeconds(timeStr string, expire int64) (int64, error) {
	const timeFormat = time.RFC3339
	tm, err := time.Parse(timeFormat, timeStr)
	if err != nil {
		return expire, err
	}
	return tm.Unix() + expire - time.Now().Unix(), nil
}

// Schedule arranges a periodical execution of function f with a goroutine.
// In this implementation delay time starts counting once the function
// call is completed.
func Schedule(f func(), delay time.Duration) chan bool {
	stop := make(chan bool)
	go func() {
		for {
			f()
			select {
			case <-time.Tick(delay):
			case <-stop:
				return
			}
		}
	}()
	return stop
}

// ServeStaticFile reads the file and serves it via specified
// http.ResponseWriter. If contentType is not an empty string,the
// corresponding header is set in http.ResponseWriter.
func ServeStaticFile(w http.ResponseWriter, path string) error {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = w.Write(file)
	return err
}
