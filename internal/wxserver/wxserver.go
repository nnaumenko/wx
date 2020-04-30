/*
* Copyright (C) 2020 Nick Naumenko (https://gitlab.com/nnaumenko)
* All rights reserved.
* This software may be modified and distributed under the terms
* of the MIT license. See the LICENSE file for details.
 */

package wxserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nnaumenko/wx/internal/database"
	"github.com/nnaumenko/wx/internal/util"
)

const (
	enableCORS   = true
	maxLocations = 16
	prettyJSON   = true
)

const (
	endpointMetar    string = "metar"
	endpointTaf      string = "taf"
	endpointLocation string = "location"
	endpointAll      string = "all"

	paramLocation string = "location"

	helpPath string = "help"

	staticPath string = ""
)

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Println(r.Method, r.URL, time.Now().Sub(start))
	})
}

func checkMethod(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			next.ServeHTTP(w, r)
		case http.MethodHead:
			next.ServeHTTP(w, r)
		case http.MethodOptions:
			util.ServeOptions(w, r, true, enableCORS)
		default:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			msg := fmt.Sprintf("Method %s is not allowed", r.Method)
			http.Error(w, msg, http.StatusMethodNotAllowed)
		}
	})
}

func addCorsHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enableCORS {
			util.SetCORSHeaders(w)
		}
		next.ServeHTTP(w, r)
	})
}

func serveStaticFile(w http.ResponseWriter, path string, contentType string) {
	err := util.ServeStaticFile(w, path)
	if err != nil {
		msg := fmt.Sprintf("Error serving file %s: %s", path, err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	if len(contentType) > 0 {
		w.Header().Set("Content-Type", contentType)
	}
}

func handleStaticPaths() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			serveStaticFile(w, staticPath+"index.html", "text/html; charset=utf-8")
		case "/help":
			serveStaticFile(w, staticPath+"help.html", "text/html; charset=utf-8")
		case "/help/":
			serveStaticFile(w, staticPath+"help.html", "text/html; charset=utf-8")
		default:
			msg := fmt.Sprintf("Unknown endpoint or path %s", r.URL.Path)
			http.Error(w, msg, http.StatusForbidden)
		}
	})
}

func parsePath(path string) (string, string, error) {
	p := strings.Split(path, "/")
	if len(p) < 1 {
		return "", "", errors.New("URL path is empty")
	}
	p = p[1:]              // first element is empty because path always begins with /
	if p[len(p)-1] == "" { // don't care whether URL path is terminated with / or not
		p = p[:len(p)-1]
	}
	switch len(p) {
	case 1:
		return p[0], "", nil
	case 2:
		return p[0], strings.ToUpper(p[1]), nil
	default:
		return "", "", fmt.Errorf("Unable to parse URL path %s", path)
	}
}

// QueryParameters stores the parameters submitted in the URL query.
type QueryParameters struct {
	Locations []string
}

func parseQuery(query string) (QueryParameters, error) {
	var qp QueryParameters
	q, err := url.ParseQuery(query)
	if err != nil {
		return qp, fmt.Errorf("Unable to parse URL query %s: %s", query, err)
	}
	for k, v := range q {
		switch k {
		case paramLocation:
			locations := util.ParseURLQueryList(v)
			for i := 0; i < len(locations); i++ {
				locations[i] = strings.ToUpper(locations[i])
			}
			qp.Locations = locations

		default:
			return qp, fmt.Errorf("Unknown parameter %s in URL query %s", k, query)
		}
	}
	return qp, nil
}

// HandlerContext is passed to endpoint handlers
type HandlerContext struct {
	Db  database.Database
	Log log.Logger
}

func queryDatabase(ctx *HandlerContext, endpoint string, locations []string) ([]*database.DataICAOLocation, error) {
	switch endpoint {
	case endpointMetar:
		return ctx.Db.GetMETARs(locations)
	case endpointTaf:
		return ctx.Db.GetTAFs(locations)
	case endpointLocation:
		ld, err := ctx.Db.GetICAOLocationData(locations)
		if err != nil {
			return make([]*database.DataICAOLocation, 0), err
		}
		for i := 0; i < len(ld); i++ {
			ld[i].Metar = ""
			ld[i].Taf = ""
		}
		return ld, err
	case endpointAll:
		return ctx.Db.GetICAOLocationData(locations)
	default:
		err := fmt.Errorf("Unknown Endpoint %s", endpoint)
		return make([]*database.DataICAOLocation, 0), err
	}

}

func serveMultipleLocations(ctx *HandlerContext, w http.ResponseWriter, endpoint string, qparam QueryParameters) {
	if len(qparam.Locations) > maxLocations {
		msg := fmt.Sprintf("%d location specified while maximum of %d is allowed",
			len(qparam.Locations), maxLocations)
		http.Error(w, msg, http.StatusForbidden)
		return
	}
	for _, l := range qparam.Locations {
		if !util.ValidateICAOLocation(l) {
			msg := fmt.Sprintf("Invalid ICAO location code format %s", l)
			http.Error(w, msg, http.StatusUnprocessableEntity)
			return
		}
	}
	ld, err := queryDatabase(ctx, endpoint, qparam.Locations)
	if err != nil {
		msg := fmt.Sprintf("Error retreiving data for locations %v: %s", qparam.Locations, err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	var j []byte
	if prettyJSON {
		j, err = json.MarshalIndent(ld, "", "  ")
	} else {
		j, err = json.Marshal(ld)
	}
	if err != nil {
		msg := fmt.Sprintf("Error converting to JSON: %s", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application-json")
	fmt.Fprintf(w, "%s\n", j)
}

func serveSingleLocation(ctx *HandlerContext, w http.ResponseWriter, endpoint string, location string, qparam QueryParameters) {
	if !util.ValidateICAOLocation(location) {
		msg := fmt.Sprintf("Invalid ICAO location code format %s", location)
		http.Error(w, msg, http.StatusUnprocessableEntity)
		return
	}
	ld, err := queryDatabase(ctx, endpoint, []string{location})
	if err != nil {
		msg := fmt.Sprintf("Error retreiving data for location %s: %s", location, err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	if len(ld) < 1 {
		exists, err := ctx.Db.LocationExists(location)
		if err != nil {
			msg := fmt.Sprintf("Error checking location existence %s: %s", location, err)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		if !exists {
			msg := fmt.Sprintf("Location %s is not found", location)
			http.Error(w, msg, http.StatusNotFound)
			return
		}
		ld = append(ld, &database.DataICAOLocation{Location: location})
	}
	if len(ld) > 1 {
		msg := fmt.Sprintf("Inconsistent data for ICAO location %s: %v", location, ld)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	var j []byte
	if prettyJSON {
		j, err = json.MarshalIndent(ld[0], "", "  ")
	} else {
		j, err = json.Marshal(ld[0])
	}
	if err != nil {
		msg := fmt.Sprintf("Error converting to JSON: %s", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application-json")
	fmt.Fprintf(w, "%s\n", j)
}

func handleEndpoints(ctx *HandlerContext) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint, locationSingle, err := parsePath(r.URL.Path)
		if err != nil {
			msg := fmt.Sprintf("Error parsing path: %s", err.Error())
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		queryParam, err := parseQuery(r.URL.RawQuery)
		if err != nil {
			msg := fmt.Sprintf("Error parsing query: %s", err.Error())
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		switch {
		case len(queryParam.Locations) > 0 && len(locationSingle) == 0:
			serveMultipleLocations(ctx, w, endpoint, queryParam)
		case len(queryParam.Locations) == 0 && len(locationSingle) > 0:
			serveSingleLocation(ctx, w, endpoint, locationSingle, queryParam)
		case len(queryParam.Locations) == 0 && len(locationSingle) == 0:
			http.Error(w, "Location not specified",
				http.StatusUnprocessableEntity)
			return
		default:
			msg := fmt.Sprintf(
				"Single location %s and multiple locations %v "+
					"must not be specified in the same request",
				locationSingle, queryParam.Locations)
			http.Error(w, msg, http.StatusUnprocessableEntity)
			return
		}
	})
}

func middleware(next http.Handler) http.Handler {
	return logRequest(checkMethod(addCorsHeaders(next)))
}

// SetupHandlers adds handlers to mux
func SetupHandlers(mux *http.ServeMux, ctx *HandlerContext) {
	mux.Handle("/", middleware(handleStaticPaths()))
	mux.Handle("/"+helpPath+"/", middleware(handleStaticPaths()))
	mux.Handle("/"+helpPath, middleware(handleStaticPaths()))

	mux.Handle("/"+endpointMetar+"/", middleware(handleEndpoints(ctx)))
	mux.Handle("/"+endpointTaf+"/", middleware(handleEndpoints(ctx)))
	mux.Handle("/"+endpointLocation+"/", middleware(handleEndpoints(ctx)))
	mux.Handle("/"+endpointAll+"/", middleware(handleEndpoints(ctx)))
	mux.Handle("/"+endpointMetar, middleware(handleEndpoints(ctx)))
	mux.Handle("/"+endpointTaf, middleware(handleEndpoints(ctx)))
	mux.Handle("/"+endpointLocation, middleware(handleEndpoints(ctx)))
	mux.Handle("/"+endpointAll, middleware(handleEndpoints(ctx)))
}
