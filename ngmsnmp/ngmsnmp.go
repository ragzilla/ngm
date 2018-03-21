package main

import ngm "github.com/ragzilla/ngm/ngmframework"
import (
	"fmt"
	"github.com/influxdata/influxdb/client/v2"
	"github.com/robryk/semaphore"
	"github.com/soniah/gosnmp"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func main() {
	f := ngm.NewNGMFramework("snmp", ngmsnmp, 8) // concurrency == 1 for testing
	f.Setup()
	f.MainLoop()
	f.Finish()
}

func ngmsnmp(j *ngm.NgmJob) bool {
	// switch it, switch it good
	// TODO: validate params
	switch j.Cmd {
	case "if-mib":
		// ifmib it
		return ifmib(j)
	default:
		fmt.Printf("ngmsnmp: unhandled command: %#v\n", j)
		return false
	}
}

type ifmibResult struct {
	future chan map[string]interface{}
	result map[string]interface{}
}

func getMapResult(k string, r map[string]*ifmibResult, v ...string) float64 {
	for _, s := range v {
		if r, ok := r[s].result[k]; ok {
			return float64(r.(uint64))
		}
	}
	return 0.0
}

func ifmib(j *ngm.NgmJob) bool {
	mibs := [...]string{
		"ifName", "ifType",
		"ifInOctets", "ifInUcastPkts", "ifInDiscards", "ifInErrors",
		"ifOutOctets", "ifOutUcastPkts", "ifOutDiscards", "ifOutErrors",
		"ifInMulticastPkts", "ifInBroadcastPkts", "ifOutMulticastPkts", "ifOutBroadcastPkts",
		"ifHCInOctets", "ifHCInUcastPkts", "ifHCInMulticastPkts", "ifHCInBroadcastPkts",
		"ifHCOutOctets", "ifHCOutUcastPkts", "ifHCOutMulticastPkts", "ifHCOutBroadcastPkts",
	}
	results := make(map[string]*ifmibResult)
	// walk all mibs mentioned in mibs
	c := 8
	if t, ok := j.Params["concurrency"]; ok {
		if ti, err := strconv.ParseUint(t, 10, 0); err == nil {
			c = int(ti)
		}
	}
	j.Params["concurrency"] = fmt.Sprintf("%d", c)
	lock := semaphore.New(c)
	for _, k := range mibs {
		results[k] = new(ifmibResult)
		results[k].future = bulkwalk(j, k, lock)
	}
	// pull in results from walks
	for k := range results {
		results[k].result = <-results[k].future
	}

	j.Result = make(map[string]*client.Point)

	// fmt.Printf("procesing for %#v\n", j)
	// fmt.Printf("ifname: %#v\n", results["ifName"])
	// fmt.Printf("iftype: %#v\n", results["ifType"])

	for k, v := range results["ifName"].result {
		// fmt.Printf("processing %#v/%#v\n", k, v)
		// check type, skip ones we don't care about
		// skip if no index in ifType
		if _, ok := results["ifType"].result[k]; !ok {
			continue
		} // ios-xr doesn't always give us a full mib
		t := reflect.ValueOf(results["ifType"].result[k]).Int()
		// skip vlan- pointless crap
		if strings.HasPrefix(v.(string), "VLAN-") || strings.HasPrefix(v.(string), "dwdm") {
			continue
		}
		switch t {
		case 1: // other, includes control plane and Null
		case 6: // ethernetCsmacd
		case 23: // ppp
		// excluding sonet and aal5, need to identify workaround for duplicate ifName in aal5 type
		// case "39": // sonet
		// 	v = v + "-sonet"
		// case "49": // aal5
		// 	v = v + "-aal5"
		case 53: // propVirtual, includes EOBC
		case 108: // pppMultilinkBundle
		case 131: // tunnel
		case 135: // l2vlan (subinterfaces)
		case 166: // mpls, set series name appropriately
			v = v.(string) + "-mpls"
		default: // if not one of our supported types, continue/skip
			// fmt.Printf("skipping ifName k: %v | v: %v | t: %v\n", k, v, t)
			continue
		}
		// fmt.Printf("ifName k: %v | v: %v | t: %v\n", k, v, t)
		tags := map[string]string{"hostname": j.Params["hostname"], "agent_host": j.Params["host"], "interface": v.(string)}
		fields := map[string]interface{}{
			"ifInOctets":         getMapResult(k, results, "ifHCInOctets", "ifInOctets"),
			"ifInUcastPkts":      getMapResult(k, results, "ifHCInUcastPkts", "ifInUcastPkts"),
			"ifInDiscards":       getMapResult(k, results, "ifInDiscards"),
			"ifInErrors":         getMapResult(k, results, "ifInErrors"),
			"ifOutOctets":        getMapResult(k, results, "ifHCOutOctets", "ifOutOctets"),
			"ifOutUcastPkts":     getMapResult(k, results, "ifHCOutUcastPkts", "ifOutUcastPkts"),
			"ifOutDiscards":      getMapResult(k, results, "ifOutDiscards"),
			"ifOutErrors":        getMapResult(k, results, "ifOutErrors"),
			"ifInMulticastPkts":  getMapResult(k, results, "ifHCInMulticastPkts", "ifInMulticastPkts"),
			"ifInBroadcastPkts":  getMapResult(k, results, "ifHCInBroadcastPkts", "ifInBroadcastPkts"),
			"ifOutMulticastPkts": getMapResult(k, results, "ifHCOutMulticastPkts", "ifOutMulticastPkts"),
			"ifOutBroadcastPkts": getMapResult(k, results, "ifHCOutBroadcastPkts", "ifOutBroadcastPkts"),
		}
		pt, err := client.NewPoint("ifMIB", tags, fields, j.QueueTime)
		if err != nil {
			panic(err)
		}
		j.Result[v.(string)] = pt
		// fmt.Printf("Added point: %s\n", pt)
	}
	// fmt.Printf("%#v", j.Result)
	return true
}

var mibToOid = map[string]string{
	"ifType":               ".1.3.6.1.2.1.2.2.1.3",
	"ifInOctets":           ".1.3.6.1.2.1.2.2.1.10",
	"ifInUcastPkts":        ".1.3.6.1.2.1.2.2.1.11",
	"ifInDiscards":         ".1.3.6.1.2.1.2.2.1.13",
	"ifInErrors":           ".1.3.6.1.2.1.2.2.1.14",
	"ifOutOctets":          ".1.3.6.1.2.1.2.2.1.16",
	"ifOutUcastPkts":       ".1.3.6.1.2.1.2.2.1.17",
	"ifOutDiscards":        ".1.3.6.1.2.1.2.2.1.19",
	"ifOutErrors":          ".1.3.6.1.2.1.2.2.1.20",
	"ifName":               ".1.3.6.1.2.1.31.1.1.1.1",
	"ifInMulticastPkts":    ".1.3.6.1.2.1.31.1.1.1.2",
	"ifInBroadcastPkts":    ".1.3.6.1.2.1.31.1.1.1.3",
	"ifOutMulticastPkts":   ".1.3.6.1.2.1.31.1.1.1.4",
	"ifOutBroadcastPkts":   ".1.3.6.1.2.1.31.1.1.1.5",
	"ifHCInOctets":         ".1.3.6.1.2.1.31.1.1.1.6",
	"ifHCInUcastPkts":      ".1.3.6.1.2.1.31.1.1.1.7",
	"ifHCInMulticastPkts":  ".1.3.6.1.2.1.31.1.1.1.8",
	"ifHCInBroadcastPkts":  ".1.3.6.1.2.1.31.1.1.1.9",
	"ifHCOutOctets":        ".1.3.6.1.2.1.31.1.1.1.10",
	"ifHCOutUcastPkts":     ".1.3.6.1.2.1.31.1.1.1.11",
	"ifHCOutMulticastPkts": ".1.3.6.1.2.1.31.1.1.1.12",
	"ifHCOutBroadcastPkts": ".1.3.6.1.2.1.31.1.1.1.13",
}

func bulkwalk(j *ngm.NgmJob, mib string, lock *semaphore.Semaphore) chan map[string]interface{} {
	// returns a future to bulkwalk a tree
	snmp := &gosnmp.GoSNMP{
		Port:      161,
		Community: j.Params["community"],
		Version:   gosnmp.Version2c,
		Target:    j.Params["host"],
		Timeout:   time.Duration(5) * time.Second,
	}
	future := make(chan map[string]interface{})
	go func() {
		if _, ok := mibToOid[mib]; !ok {
			fmt.Printf("bulkwalk[%s]: unknown mib\n", mib)
			future <- map[string]interface{}{}
			return
		}
		// walk the mib
		lock.Acquire(1)
		snmp.Connect()
		// fmt.Printf("%v/%v using laddr %#v\n", snmp.Target, mib, snmp.Conn.LocalAddr().String())
		oid := mibToOid[mib]
		retval := make(map[string]interface{})
		result, err := snmp.BulkWalkAll(oid) // once
		if err != nil {
			result, err = snmp.BulkWalkAll(oid) // twice
			if err != nil {
				result, err = snmp.BulkWalkAll(oid) // third time's a charm!
				if err != nil {
					lock.Release(1)
					snmp.Conn.Close()
					future <- retval
					return
				}
			}
		}
		for _, v := range result {
			idx := strings.TrimPrefix(v.Name, oid+".")
			switch t := v.Value.(type) {
			case int, int64:
				retval[idx] = reflect.ValueOf(t).Int()
			case uint, uint64:
				retval[idx] = reflect.ValueOf(t).Uint()
			case string:
				retval[idx] = t
			case []uint8:
				retval[idx] = string(t)
			case nil:
				// empty case, decode error
			default:
				fmt.Printf("unknown data[%v]: %v = %v: %v [%v]\n", oid, idx, v.Type, t, reflect.TypeOf(t))
			}
		}
		// fmt.Printf("retval(%v/%v): %v\n", mib, oid, retval)
		lock.Release(1)
		snmp.Conn.Close()
		future <- retval
	}()
	return future
}
