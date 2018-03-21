package ngmframework

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/influxdata/influxdb/client/v2"
	"github.com/nats-io/go-nats"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ngmFramework struct {
	queue       string
	inqueue     string
	outqueue    string
	workername  string
	hostname    string
	test        bool
	nats        *nats.Conn
	quit        chan os.Signal
	gather      chan bool
	gathered    chan bool
	workout     chan *NgmJob
	workin      chan *NgmJob
	callback    func(*NgmJob) bool
	concurrency int
}

type NgmJob struct {
	JobName   string
	Cmd       string
	Params    map[string]string
	Result    map[string]*client.Point
	Worker    string
	QueueTime time.Time
	StartTime time.Time
	EndTime   time.Time
}

func NewNGMFramework(queue string, callback func(*NgmJob) bool, concurrency int) *ngmFramework {
	f := new(ngmFramework)

	flag.BoolVar(&f.test, "test", true, "Run in test environment") // defaulting to true
	flag.Parse()

	if f.test {
		fmt.Println("Test:", f.test)
	}

	f.queue = queue
	f.inqueue = fmt.Sprintf("%s.request", queue)
	f.outqueue = fmt.Sprintf("%s.result", queue)

	if f.test {
		f.inqueue += ".test"
		f.outqueue += ".test"
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	f.hostname = hostname
	f.workername = fmt.Sprintf("%s/%s/%d", hostname, queue, os.Getpid())
	f.nats = nil
	f.quit = make(chan os.Signal, 1)
	f.workout = make(chan *NgmJob, 0)
	f.workin = make(chan *NgmJob, concurrency) // buffer
	f.gather = make(chan bool, 0)
	f.gathered = make(chan bool, concurrency)
	f.callback = callback
	f.concurrency = concurrency
	signal.Notify(f.quit, os.Interrupt, syscall.SIGTERM)
	return f
}

func (f *ngmFramework) Setup() {
	// TODO: send a process started message
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		panic(err)
	}
	f.nats = nc
}

func (f *ngmFramework) MainLoop() {
	// nats async subscriber
	sub, err := f.nats.QueueSubscribe(f.inqueue, f.inqueue, func(m *nats.Msg) {
		j := new(NgmJob)
		err := json.Unmarshal(m.Data, &j)
		if err != nil {
			fmt.Printf("Error unmarshalling: %v\n", err)
		} else {
			// TODO: export a job started message/log
			// TODO: validate schema
			j.Worker = f.workername
			f.workout <- j
		}
	})
	if err != nil {
		panic(err)
	}
	for i := 0; i < f.concurrency; i++ {
		go f.worker(i)
	}
	for { // loop until f.quit is signalled
		select {
		case res := <-f.workin:
			f.publish(res)
		case _ = <-f.quit:
			sub.Unsubscribe()
			for i := 0; i < f.concurrency; i++ {
				f.gather <- true
			}
			for i := 0; i < f.concurrency; i++ {
				<-f.gathered
			}
			for {
				select {
				case res := <-f.workin:
					f.publish(res)
				default:
					fmt.Printf("quitting...\n")
					return
				}
			}
		}
	}
}

func (f *ngmFramework) publish(res *NgmJob) {
	var buffer bytes.Buffer
	for _, pt := range res.Result {
		buffer.WriteString(pt.String() + "\n")
	}
	f.nats.Publish(f.outqueue, buffer.Bytes())
}

func (f *ngmFramework) worker(i int) {
	// loop forever
	// TODO: log idle time, add point to job export
	idle := time.Now()
	for {
		select {
		case j := <-f.workout:
			idletime := int64(time.Since(idle) / time.Millisecond)
			j.StartTime = time.Now()
			f.callback(j)
			j.EndTime = time.Now()
			elapsed := int64(j.EndTime.Sub(j.StartTime) / time.Millisecond)
			//fmt.Printf("worker: %d | idletime: %d | elapsed: %v\n", i, idletime, elapsed)

			// add point
			tags := map[string]string{"hostname": j.Params["hostname"], "agent_host": j.Params["host"], "poller": f.hostname, "queue": f.queue}
			fields := map[string]interface{}{"elapsed": elapsed, "idletime": idletime}
			pt, err := client.NewPoint("ngmStatistics", tags, fields, j.QueueTime)
			if err != nil {
				panic(err)
			}
			j.Result["ngmStatistics"] = pt
			f.workin <- j
			idle = j.EndTime
		case <-f.gather:
			fmt.Printf("worker %d exiting...\n", i)
			f.gathered <- true
			return
		}
	}
}

func (f *ngmFramework) Finish() {
	// TODO: send a process exited message
	if f.nats != nil {
		f.nats.Close()
	}
}
