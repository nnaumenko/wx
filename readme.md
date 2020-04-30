# JSON REST API weather service

JSON REST API to provide METAR, TAF and location info for ICAO location code.

[Live demo](https://wx.void.fo)

This project is still an early draft, more features coming soon.

## API

[Quick reference](https://wx.void.fo/help)

[API Description on Swagger](https://app.swaggerhub.com/apis/nnaumenko/wx.void.fo/0.1)

## Description

In this implementation the weather and location data are stored in Redis.

Consists of two microservices: 
* wx-server: web server to serve requested JSONs
* wx-update: data updater to automatically acquire the data from [Text Data Server on AviationWeather](https://www.aviationweather.gov/dataserver) and Location data from [OurAirports](https://ourairports.com/data/).

Currently provides only raw / undecoded METARs and TAFs.