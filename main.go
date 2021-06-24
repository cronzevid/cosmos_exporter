package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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

type addrBookJson struct {
	Addrs []addrBook `json:"addrs"`
}

type addrBook struct {
	Addr addrData `json:"addr"`
}

type addrData struct {
	Id string `json:"id"`
}

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
var appHost = flag.String("app-host", "127.0.0.1", "Host of exposed API")
var appPort = flag.String("app-port", ":1317", "Port of exposed API")

func callApi(apiHost string, callType string, metric prometheus.Gauge) {

	apiRoute := "blocks/latest"

	for {
		fullApiRoute := fmt.Sprintf("http://%s/%s", apiHost, apiRoute)
		response, err := http.Get(fullApiRoute)
		if err != nil {
			log.Printf("%s", err)
		} else {
			contents, err := ioutil.ReadAll(response.Body)
			if err != nil {
				log.Printf("%s", err)
			}

			apiResponse := blocksLatest{}
			json.Unmarshal([]byte(contents), &apiResponse)

			if callType == "block" {
				height, _ := strconv.ParseFloat(apiResponse.Block.Header.Height, 64)

				metric.Set(height)
			} else if callType == "time" {
				blockDateApi := apiResponse.Block.Header.Time

				blockDate, _ := time.Parse(time.RFC3339Nano, blockDateApi)
				blockSec := blockDate.UnixNano()

				now := time.Now()
				curNanoSec := now.UnixNano()

				metric.Set(float64(curNanoSec - blockSec))
			}

			response.Body.Close()
		}

		time.Sleep(5 * time.Second)
	}
}

func getPeerAmount(addrBook string, metric prometheus.Gauge) {
	for {
		jsonFile, err := os.Open(addrBook)

		if err != nil {
			fmt.Println(err)
		}

		addrJsonBytes, _ := ioutil.ReadAll(jsonFile)

		addrJson := addrBookJson{}
		json.Unmarshal([]byte(addrJsonBytes), &addrJson)

		jsonFile.Close()

		metric.Set(float64(len(addrJson.Addrs)))
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

	go callApi(*appHost+*appPort, "block", blockNum)
	go callApi(*appHost+*appPort, "time", timeSkew)
	go getPeerAmount("/root/.gaia/config/addrbook.json", peerAmount)

	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Starting web server at %s\n", *addr)

	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Printf("http.ListenAndServer: %v\n", err)
	}

}
