This was ported from an internal repo, and import URLs for the github.com/ragzilla/ngm/ pieces may not work, I'm not sure.

Written against a much older version of go (before DSOs) which would be a much cleaner way to implement nowadays.

Written against an older version of InfluxDB (before int64 support).

* Requires NATS (gnatsd tested) running on localhost, and a telegraf server.
* Compile sendngmsnmp (demo program) and ngmsnmp. 
* Run ngmsnmp as a daemon and it'll listen on the NATS queue snmp.request
* Run sendngmsnmp (or your own job submitter) from cron to send jobs into snmp.request. The design idea behind this internally was putting hosts on a consistent hash ring and submitting jobs across the 60 second polling window so we'd be submitting 50 jobs/second instead of 3000 jobs all at once. Using NATS (gnatsd) should allow for horizontal scaling as needed (just add more ngmsnmp processes listening on the same queue).
* Results are posted in InfluxDB line format to queue snmp.result, have telegraf listen on that queue and insert into InfluxDB (handles bulking etc so the poller can be more of a naive implementation)
