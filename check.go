package main

import (
	"code.google.com/p/gorilla/mux"
	"encoding/csv"
	"fmt"
	"github.com/miekg/dns"
	"log"
	"net/http"
	"sort"
	"strings"
	"unbound"
)

type result struct {
	name   string // name to be checked
	err    string // error from unbound (if any)
	status string // security status
	why    string // WhyBogus from unbound (DNSSEC error)
}

type AllResults struct {
	r []*result
}

func NewAllResults() *AllResults {
	a := new(AllResults)
	a.r = make([]*result, 0)
	return a
}

func (a *AllResults) Append(r *result) { a.r = append(a.r, r) }
func (a *AllResults) Len() int         { return len(a.r) }

// Sort on status (bogus is with a 'b' so it will end up first)
func (a *AllResults) Less(i, j int) bool { return a.r[i].status < a.r[j].status }
func (a *AllResults) Swap(i, j int)      { a.r[i], a.r[j] = a.r[j], a.r[i] }

// Create a string slice from *result for printing
func (r *result) serialize() []string {
	if r != nil {
		s := make([]string, 4)
		s[0] = r.name
		s[1] = r.err
		s[2] = r.status
		s[3] = r.why
		return s
	}
	return nil
}

// Create HTML from *result (not used yet)
func (r *result) serializeToHTML() {
	// ...
}

// TODO(mg)
func preCheckHandler(w http.ResponseWriter, r *http.Request) {
	return
}

func unboundcheck(u *unbound.Unbound, zone string) *result {
	zone = strings.TrimSpace(zone)
	r := new(result)
	r.name = zone
	res, err := u.Resolve(zone, dns.TypeNS, dns.ClassINET)
	log.Printf("checking %s\n", zone)
	if err != nil {
		r.err = err.Error()
		return r
	}
	if res.HaveData {
		if res.Secure {
			r.status = "secure"
		} else if res.Bogus {
			r.status = "bogus"
			r.why = res.WhyBogus
		} else {
			r.status = "insecure"
		}
	} else {
		r.err = "nodata"
	}
	return r
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	zone := vars["domain"]

	u := unbound.New()
	defer u.Destroy()
	setupUnbound(u)
	result := unboundcheck(u, zone)
	o := csv.NewWriter(w)
	if e := o.Write(result.serialize()); e != nil {
		log.Printf("Failed to write csv: %s\n", e.Error())
	}
	o.Flush()
}

func parseHandlerCSV(w http.ResponseWriter, r *http.Request) {
	f, _, err := r.FormFile("domainlist")
	if err != nil {
		fmt.Println(err)
		return
	}
	u := unbound.New()
	defer u.Destroy()
	setupUnbound(u)

	v := csv.NewReader(f)
	o := csv.NewWriter(w)
	record, err := v.Read()
	all := NewAllResults()
	for err == nil {
		for _, r := range record {
			result := unboundcheck(u, r)
			log.Printf("%v\n", result)
			all.Append(result)
		}
		record, err = v.Read()
	}
	sort.Sort(all)
	for _, r := range all.r {
		if e := o.Write(r.serialize()); e != nil {
			log.Printf("Failed to write csv: %s\n", e.Error())
		}
		o.Flush()
	}
}

func form(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, `
<html>
	<head>
	<title>Portfolio check</title>
	</head>
	<body>
	<h1>Selecteer een <em>CSV</em> bestand met domein namen:</h1>
	<form action="http://localhost:8080/upload" method="POST" enctype="multipart/form-data">
	<input type="file" name="domainlist">
	<input type="submit" value="Controleer">
	</form>
	<h2>FAQ</h2>
	<dl>
	<dt>Hoe wordt er gecontroleeerd?</dt>
	<dd>U uploadt een CSV bestand met domein namen. Alle namen worden gecontroleerd, dus ook niet-.nl domein namen.
	<p>
	Er wordt een secure lookup via Unbound uitgevoerd. Er zal dus een <em>willekeurige</em> selectie van nameservers
	plaatsvinden en is er is geen garantie dat al uw slaves nameservers worden gecheckt.</dd>

	<dt>Hoe ziet de uitvoer eruit?</dt>
	<dd>De uitvoer van deze check is:
	<p>
	<code>
		domeinnaam, DNS error, security status, uitgebreide error als bogus
	</code>
	</p>

	De security status kan zijn
	<ul>
	<li><b>secure</b>: de domein naam is correct beveiligd met DNSSEC</li>
	<li><b>bogus</b>: de domein naam is <em>niet</em> correct beveiligd met DNSSEC</li>
	<li><b>insecure</b>: de domein naam is niet beveiligd met DNSSEC</li>

	</ul>
	<p/>
	De DNS error als Unbound geen informatie kan vinden in het DNS, er zal hier dan de
	string <b>nodata</b> worden weergegeven.
	<p/>
	De uitvoer is gesorteerd op de security status, dus alle <em>b</em>ogus domeinen komen vooraan te staan.

	</dd>

	<dt>Kan er ook een enkele domein naam worden gecontroleerd?</dt>
	<dd>Uiteraard is dat mogelijk door een bestand te uploaden dat maar 1 domein naam bevat. Of u kunt
	de volgende URL gebruiken die een RESTful-achtige interface aanbiedt:
	<p>
	<a href="http://localhost:8080/check/">localhost:8080/check/domeinnaam</a>
	</p>
	Bv: 
	<a href="http://localhost:8080/check/example.nl">localhost:8080/check/example.nl</a>
	<p>
	De uitvoer daarvan is gelijk aan de portfolio-check uitvoer (CSV).
	</p>
	<dd>Welke software is gebruikt?</dd>
	Deze portfolio-check gebruikt:
	<ul>
		<li>Libunbound van <a href="http://www.nlnetlabs.nl">NLnet Labs</a></li>
		<li>De taal <a href="http://www.golang.org">Go</a></li>
	</ul>
	De complete software is te vinden op <a href="http://github.com/SIDN/unboundcheck>github.com/SIDN/unboundcheck</a>.
	</dd>
	</dl>
	<h2>Disclaimer</h2>
	Dit is beta software! 
	</body>
</html>`)
}

func main() {
	router := mux.NewRouter()
	//router.HandleFunc("/precheck/{domain}/{anchor}", preCheckHandler) // TODO(mg)
	router.HandleFunc("/check/{domain}", checkHandler)
	router.HandleFunc("/upload", parseHandlerCSV)
	router.HandleFunc("/form", form)
	http.Handle("/", router)

	e := http.ListenAndServe(":8080", nil)
	if e != nil {
		log.Fatal("ListenAndServe: ", e)
	}
}

// Setup the resolver and add the root's trust anchor
func setupUnbound(u *unbound.Unbound) {
	u.ResolvConf("/etc/resolv.conf")
	u.AddTaFile("Kroot.key")
}
