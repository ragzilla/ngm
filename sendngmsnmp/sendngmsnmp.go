package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/nats-io/go-nats"
	"github.com/ragzilla/ngm/ngmframework"
	"time"
)

func main() {
	hosts := map[string][]string{
		"chi.cr1":             []string{"192.0.2.42", "4"},
		"demo1.rr":            []string{"192.0.2.18", "4"},
		"demo1.olddr1":        []string{"192.0.2.91", "4"},
		"demo1.olddr2":        []string{"192.0.2.92", "4"},
		"demo1.oldcar1":       []string{"192.0.2.2", "4"},
		"demo1.oldcar2":       []string{"192.0.2.6", "4"},
		"demo1.dr1":           []string{"192.0.2.148", "4"},
		"demo1.dr2":           []string{"192.0.2.149", "4"},
		"demo1.car1":          []string{"192.0.2.152", "4"},
		"demo1.car2":          []string{"192.0.2.153", "4"},
		"demo1.car3":          []string{"192.0.2.5", "4"},
		"demo1.mer1":          []string{"192.0.2.247", "4"},
		"demo1.mer2":          []string{"192.0.2.246", "4"},
		"demo2.rr":            []string{"192.0.2.128", "4"},
		"demo2.dr1":           []string{"192.0.2.132", "4"},
		"demo2.dr2":           []string{"192.0.2.133", "4"},
		"demo2.car1":          []string{"192.0.2.136", "4"},
		"demo2.car2":          []string{"192.0.2.137", "4"},
		"demo1.clu1":          []string{"192.0.2.3", "4"},
		"demo1.clu2":          []string{"192.0.2.4", "4"},
	}

	j := new(ngmframework.NgmJob)
	j.QueueTime = time.Now()
	j.Cmd = "if-mib"
	j.Params = make(map[string]string)
	j.Params["host"] = ""
	j.Params["hostname"] = ""
	j.Params["community"] = "public"

	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		panic(err)
	}
	defer nc.Close()

	for k, v := range hosts {
		j.JobName = fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s-snmp-if-mib", v))))
		j.Params["host"] = v[0]
		j.Params["concurrency"] = v[1]
		j.Params["hostname"] = k
		js, err := json.Marshal(&j)
		if err != nil {
			panic(err)
		}
		nc.Publish("snmp.request", js)
	}
}
