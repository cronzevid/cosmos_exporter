package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type blocksLatest struct {
	Block innerBlock `json:"block"`
}

type innerBlock struct {
	Header headerData `json:"header"`
}

type headerData struct {
	Height string `json:"height"`
	Time   string `json:"time"`
}

type validatorSetsLatest struct {
	Result resultContent `json:"result"`
}

type resultContent struct {
	Validators []validator `json:"validators"`
}

type validator struct {
	Address string `json:"address"`
}

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
var appHost = flag.String("app-host", "127.0.0.1", "Host of exposed API")
var appPort = flag.String("app-port", ":1317", "Port of exposed API")

func getBlockNum(apiHost string, apiRoute string, metric prometheus.Gauge) {

	for {
		fullApiRoute := fmt.Sprintf("http://%s/%s", apiHost, apiRoute)
		response, err := http.Get(fullApiRoute)
		if err != nil {
			log.Printf("%s", err)
		} else {
			defer response.Body.Close()
			contents, err := ioutil.ReadAll(response.Body)
			if err != nil {
				log.Printf("%s", err)
			}

			apiResponse := blocksLatest{}
			json.Unmarshal([]byte(contents), &apiResponse)

			height, _ := strconv.ParseFloat(apiResponse.Block.Header.Height, 64)

			metric.Set(height)
		}

		time.Sleep(5 * time.Second)
	}
}

func getTimeSkew(apiHost string, apiRoute string, metric prometheus.Gauge) {
	for {
		fullApiRoute := fmt.Sprintf("http://%s/%s", apiHost, apiRoute)
		response, err := http.Get(fullApiRoute)
		if err != nil {
			log.Printf("%s", err)
		} else {
			defer response.Body.Close()
			contents, err := ioutil.ReadAll(response.Body)
			if err != nil {
				log.Printf("%s", err)
			}

			apiResponse := blocksLatest{}
			json.Unmarshal([]byte(contents), &apiResponse)

			blockDateApi := apiResponse.Block.Header.Time

			blockDate, err := time.Parse(time.RFC3339Nano, blockDateApi)
			blockSec := blockDate.UnixNano()

			now := time.Now()
			curNanoSec := now.UnixNano()

			metric.Set(float64(curNanoSec - blockSec))
		}
		time.Sleep(5 * time.Second)
	}
}

func getPeerAmount(apiHost string, apiRoute string, metric prometheus.Gauge) {
	for {
		fullApiRoute := fmt.Sprintf("http://%s/%s", apiHost, apiRoute)
		response, err := http.Get(fullApiRoute)
		if err != nil {
			log.Printf("%s", err)
		} else {
			defer response.Body.Close()
			contents, err := ioutil.ReadAll(response.Body)
			if err != nil {
				log.Printf("%s", err)
			}

			apiResponse := validatorSetsLatest{}
			json.Unmarshal([]byte(contents), &apiResponse)

			validators := apiResponse.Result.Validators

			metric.Set(float64(len(validators)))
		}
		time.Sleep(5 * time.Second)
	}
}

func main() {
	flag.Parse()

	blockNum := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cosmos_block_number",
		Help: "Number of the latest block in chain",
	})
	timeSkew := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cosmos_block_time_skew",
		Help: "Difference between current timestamp and block timestamp",
	})
	peerAmount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cosmos_node_peers",
		Help: "Amount of chain peers",
	})

	prometheus.MustRegister(blockNum)
	prometheus.MustRegister(timeSkew)
	prometheus.MustRegister(peerAmount)

	go getBlockNum(*appHost+*appPort, "blocks/latest", blockNum)
	go getTimeSkew(*appHost+*appPort, "blocks/latest", timeSkew)
	go getPeerAmount(*appHost+*appPort, "/validatorsets/latest", peerAmount)

	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Starting web server at %s\n", *addr)

	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Printf("http.ListenAndServer: %v\n", err)
	}

}
