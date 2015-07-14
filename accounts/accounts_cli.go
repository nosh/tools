package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/codegangsta/cli"
	"github.com/howeyc/gopass"
)

// Represents the user running the audit on the accounts that they have access to via permissons
type admin struct {
	host   string
	client *http.Client
	User   struct {
		Id    string `json:"userid"`
		Name  string `json:"username"`
		Token string `json:"-"`
	} `json:"administrator"`
	Accounts `json:"administeredAccounts"`
}

// Account details that we will audit
type Account struct {
	Id      string `json:"userid"`
	Profile struct {
		FullName string `json:"fullName"`
		Patient  struct {
			Bday string `json:"birthday"`
			Dday string `json:"diagnosisDate"`
		} `json:"patient"`
	} `json:"patientProfile"`
	Perms      interface{} `json:"permissons"`
	LastUpload string      `json:"lastupload"`
}

type AccountBasics struct {
	Id         string `json:"userid"`
	FullName   string `json:"fullName"`
	LastUpload string `json:"lastUpload"`
}

type Accounts []*Account
type Basics []*AccountBasics

func (a Accounts) ToBasics() Basics {
	var basics Basics
	for i := range a {
		basics = append(basics, &AccountBasics{Id: a[i].Id, FullName: a[i].Profile.FullName, LastUpload: a[i].LastUpload})
	}
	return basics
}

// from http://golang.org/pkg/sort/#example_Interface
func (a Accounts) sortByName() Accounts {
	name := func(a1, a2 *Account) bool {
		//alpha order
		return a1.Profile.FullName > a2.Profile.FullName
	}
	By(name).sort(a)
	return a
}

func (a Accounts) sortByUpload() Accounts {
	upload := func(a1, a2 *Account) bool {
		//return accounts that have a `LastUpload` first
		return a1.LastUpload > a2.LastUpload
	}
	By(upload).sort(a)
	return a
}

func (a Accounts) hasUploads() Accounts {
	var withUploads Accounts

	for i := range a {
		if a[i].LastUpload != "" {
			withUploads = append(withUploads, a[i])
		}
	}
	return withUploads
}

type By func(a1, a2 *Account) bool

func (by By) sort(accounts Accounts) {
	as := &accountSorter{
		accounts: accounts,
		by:       by,
	}
	sort.Sort(as)
}

type accountSorter struct {
	accounts Accounts
	by       func(a1, a2 *Account) bool
}

//All part of the sort interface
func (s *accountSorter) Len() int           { return len(s.accounts) }
func (s *accountSorter) Swap(i, j int)      { s.accounts[i], s.accounts[j] = s.accounts[j], s.accounts[i] }
func (s *accountSorter) Less(i, j int) bool { return s.by(s.accounts[i], s.accounts[j]) }

var (
	prodHost    = "https://api.tidepool.io"
	stagingHost = "https://staging-api.tidepool.io"
	develHost   = "https://devel-api.tidepool.io"
	localHost   = "http://localhost:8009"
)

func main() {

	app := cli.NewApp()

	app.Name = "Accounts-Management"
	app.Usage = "Allows the bulk management of tidepool accounts"
	app.Version = "0.0.1"
	app.Author = "Jamie Bate"
	app.Email = "jamie@tidepool.org"

	app.Commands = []cli.Command{

		//e.g. audit -u admin@place.org -e prod
		//e.g. audit -u admin@place.org -e prod -r ./audit_admin@place.org_accounts_prod_2015-07-01T03:00:58Z.txt
		{
			Name:      "audit",
			ShortName: "a",
			Usage:     "audit all accounts that you have permisson to access",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "e, env",
					Usage: "the environment your running against. Options are local, devel, staging and prod",
				},
				cli.StringFlag{
					Name:  "u, username",
					Usage: "your tidepool username that has access to the accounts e.g. admin@tidepool.org",
				},
				cli.StringFlag{
					Name:  "r, reportpath",
					Usage: "rerun the process on an existing report that you have given the path too e.g -r ./accountsAudit_2015-06-26 08:17:29.445414108 +1200 NZST.txt",
				},
			},
			Action: auditAccounts,
		},
		{
			Name:      "audit-report",
			ShortName: "ar",
			Usage:     "prepare report from a raw audit file",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "r, reportpath",
					Usage: "raw audit file e.g -r ./accountsAudit_2015-06-26 08:17:29.445414108 +1200 NZST.txt",
				},
				cli.BoolFlag{
					Name:  "u, uploads",
					Usage: "show those accounts with uploads",
				},
			},
			Action: prepareAuditReport,
		},
	}
	app.Run(os.Args)
}

func (a *admin) findPublicInfo(acc *Account) error {

	urlPath := a.host + fmt.Sprintf("/metadata/%s/profile", acc.Id)

	req, err := http.NewRequest("GET", urlPath, nil)
	if err != nil {
		return err
	}
	req.Header.Add("x-tidepool-session-token", a.User.Token)

	res, err := a.client.Do(req)

	if err != nil {
		return errors.New("Could attempt to find profile: " + err.Error())
	}

	switch res.StatusCode {
	case 200:
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		json.Unmarshal(data, &acc.Profile)
		return nil
	default:
		log.Printf("Failed finding public info [%d] for [%s]", res.StatusCode, acc.Id)
		return nil
	}
}

func (a *admin) findLastUploaded(acc *Account) error {

	urlPath := a.host + fmt.Sprintf("/query/upload/lastentry/%s", acc.Id)

	req, _ := http.NewRequest("GET", urlPath, nil)
	req.Header.Add("x-tidepool-session-token", a.User.Token)

	res, err := a.client.Do(req)

	if err != nil {
		return errors.New("Could attempt to find the last upload: " + err.Error())
	}

	switch res.StatusCode {
	case 200:
		data, err := ioutil.ReadAll(res.Body)

		if err != nil {
			log.Println("Error trying to read the last upload data", err.Error())
			return nil
		}

		json.Unmarshal(data, &acc.LastUpload)
		return nil
	default:
		log.Printf("Failed finding last upload info [%d] for [%s]", res.StatusCode, acc.Profile.FullName)
		return nil
	}
}

func (a *admin) login(un, pw string) error {

	urlPath := a.host + "/auth/login"

	req, err := http.NewRequest("POST", urlPath, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(un, pw)

	res, err := a.client.Do(req)
	if err != nil {
		return errors.New(fmt.Sprint("Login request failed", err.Error()))
	}

	switch res.StatusCode {
	case 200:
		data, _ := ioutil.ReadAll(res.Body)
		json.Unmarshal(data, &a.User)
		a.User.Token = res.Header.Get("x-tidepool-session-token")
		return nil
	default:
		return errors.New(fmt.Sprint("Login failed", res.StatusCode))
	}
}

func (a *admin) findAccounts() error {

	urlPath := a.host + fmt.Sprintf("/access/groups/%s", a.User.Id)

	req, err := http.NewRequest("GET", urlPath, nil)
	if err != nil {
		return err
	}
	req.Header.Add("x-tidepool-session-token", a.User.Token)

	res, err := a.client.Do(req)
	if err != nil {
		return errors.New("Could attempt to find administered accounts: " + err.Error())
	}

	switch res.StatusCode {
	case 200:
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		var raw map[string]interface{}

		json.Unmarshal(data, &raw)

		for key, value := range raw {
			a.Accounts = append(a.Accounts, &Account{Id: string(key), Perms: value})
		}
		return nil
	default:
		log.Println("Failed finding profiles", res.StatusCode)
		return nil
	}
}

func (a *admin) loadExistingReport(path string) error {
	log.Println("loading audit report ...")

	rf, err := os.Open(path)
	if err != nil {
		return err
	}

	jsonParser := json.NewDecoder(rf)
	if err = jsonParser.Decode(&a); err != nil {
		return err
	}
	rf.Close()
	return nil
}

func (a *admin) accountDetails() {
	var wg sync.WaitGroup

	for _, account := range a.Accounts {
		// Increment the WaitGroup counter.
		wg.Add(1)
		// Launch a goroutine to account data
		go func(account *Account) {
			// Decrement the counter when the goroutine completes.
			defer wg.Done()
			// Fetch the account data
			a.findPublicInfo(account)
		}(account)
	}
	// Wait for all fetches to complete.
	wg.Wait()
	return
}

func (a *admin) doAudit(acc *Account) error {
	return a.findLastUploaded(acc)
}

func (a *admin) audit() {

	var wg sync.WaitGroup

	for _, account := range a.Accounts {
		// Increment the WaitGroup counter.
		wg.Add(1)
		// Launch a goroutine to account data
		go func(account *Account) {
			// Decrement the counter when the goroutine completes.
			defer wg.Done()
			if err := a.findLastUploaded(account); err != nil {
				//just die
				log.Println("Failed while running findLastUploaded for account")
				//log.Println(err.Error())
				wg.Done()
				return
			}
			//time.Sleep(time.Second * 3) //pause between each
		}(account)
	}
	// Wait for all fetches to complete.
	wg.Wait()
	return
}

func setHost(targetEnv string) string {

	targetEnv = strings.ToLower(targetEnv)

	fmt.Println("Run audit against:", targetEnv)

	if targetEnv == "devel" {
		return develHost
	} else if targetEnv == "prod" {
		return prodHost
	} else if targetEnv == "staging" {
		return stagingHost
	} else if targetEnv == "local" {
		return localHost
	}
	log.Fatal("No matching environment found")
	return ""
}

// and audit will find all account linked
func auditAccounts(c *cli.Context) {

	if c.String("username") == "" {
		log.Fatal("Please specify the username with the --username or -u flag.")
	}

	if c.String("env") == "" {
		log.Fatal("Please specify the environment your running against with the --env or -e flag.")
	}

	adminUser := &admin{host: setHost(c.String("env")), client: &http.Client{}}

	fmt.Printf("Password: ")
	pass := gopass.GetPasswdMasked()

	err := adminUser.login(c.String("username"), string(pass[:]))
	if err != nil {
		log.Println(err.Error())
		return
	}

	if c.String("reportpath") == "" {

		//get accounts I can view
		log.Println("finding administered accounts ...")
		err = adminUser.findAccounts()
		if err != nil {
			log.Println(err.Error())
			return
		}
		log.Println("get users info ...")
		adminUser.accountDetails()

	} else {
		//find the accociated profiles
		log.Println("re-running audit on accounts ...")

		err = adminUser.loadExistingReport(c.String("reportpath"))
		if err != nil {
			log.Println("error loading existing report")
			log.Println(err.Error())
			return
		}
	}

	//for each account run the audit

	start := time.Now()
	const block_size = 200

	if block_size >= len(adminUser.Accounts) {
		log.Println("running audit on accounts ...")
		adminUser.audit()
		log.Printf("audit took [%f]secs", time.Now().Sub(start).Seconds())
	} else {
		log.Println("you have ", len(adminUser.Accounts), "account and ")
	}

	if len(adminUser.Accounts) > block_size {

		total := len(adminUser.Accounts)

		log.Println("building audits ...", total, "accounts")
		//split and create reports
		blocks := len(adminUser.Accounts) / block_size

		log.Println("run as", blocks, "blocks")

		var reports []Accounts
		count := 0

		for i := 0; i < blocks; i++ {
			reports = append(reports, adminUser.Accounts[count:count+block_size])
			count = count + block_size
		}
		//get the last ones
		if total > count {
			log.Println("get the last ones... from", count, "to", total)
			reports = append(reports, adminUser.Accounts[count:total])
		}

		//each block of reports
		for y := range reports {

			rpt := adminUser
			rpt.Accounts = reports[y].sortByUpload()

			jsonRpt, err := json.MarshalIndent(rpt, "", "  ")

			if err != nil {
				log.Println("error creating audit content")
				log.Println(err.Error())
				return
			}

			reportPath := fmt.Sprintf("./audit_%s_accounts_%s_part-%d_%s.txt", adminUser.User.Name, c.String("env"), y, time.Now().UTC().Format(time.RFC3339))

			f, err := os.Create(reportPath)
			if err != nil {
				log.Println("error creating audit file")
				log.Println(err.Error())
				return
			}
			defer f.Close()
			f.Write(jsonRpt)
			log.Println("done! here is the audit " + reportPath)
		}

	} else {

		log.Println("building audit ...")
		adminUser.Accounts = adminUser.Accounts.sortByUpload()

		jsonRpt, err := json.MarshalIndent(adminUser, "", "  ")
		if err != nil {
			log.Println("error creating audit content")
			log.Println(err.Error())
			return
		}

		if c.String("reportpath") == "" {

			reportPath := fmt.Sprintf("./audit_%s_accounts_%s_%s.txt", adminUser.User.Name, c.String("env"), time.Now().UTC().Format(time.RFC3339))

			f, err := os.Create(reportPath)
			if err != nil {
				log.Println("error creating audit file")
				log.Println(err.Error())
				return
			}
			defer f.Close()
			f.Write(jsonRpt)
			log.Println("done! here is the audit " + reportPath)
			return
		}

		err = ioutil.WriteFile(c.String("reportpath"), jsonRpt, 0644)
		if err != nil {
			log.Println("error updating audit file")
			log.Println(err.Error())
			return
		}

	}

	return
}

// and audit will find all account linked
func prepareAuditReport(c *cli.Context) {

	if c.String("reportpath") == "" {
		log.Fatal("Please specify the environment your running against with the --env or -e flag.")
	}

	adminUser := &admin{}
	err := adminUser.loadExistingReport(c.String("reportpath"))
	if err != nil {
		log.Println("error loading existing report")
		log.Println(err.Error())
		return
	}

	log.Println("accounts in audit", len(adminUser.Accounts))

	var toReport Basics

	if c.Bool("uploads") {
		toReport = adminUser.Accounts.hasUploads().sortByName().ToBasics()
		log.Println("accounts w uploads", len(toReport))
	} else {
		toReport = adminUser.Accounts.sortByName().ToBasics()
		log.Println("accounts w-out uploads", len(toReport))
	}

	jsonRpt, err := json.MarshalIndent(toReport, "", "")
	if err != nil {
		log.Println("error creating report content")
		log.Println(err.Error())
		return
	}

	reportPath := strings.Replace(c.String("reportpath"), "./", "./report_", 1)

	f, err := os.Create(reportPath)
	if err != nil {
		log.Println("error creating report file")
		log.Println(err.Error())
		return
	}
	defer f.Close()
	f.Write(jsonRpt)
	log.Println("done! here is the report " + reportPath)
	//see when they last uploaded

	return
}
