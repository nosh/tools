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
	"time"

	"github.com/codegangsta/cli"
	"github.com/howeyc/gopass"
)

// Represents the user running the audit on the accounts that they have access to via permissons
type admin struct {
	User struct {
		Id    string `json:"userid"`
		Name  string `json:"username"`
		Token string `json:"-"`
	} `json:"administrator"`
	Users `json:"administeredAccounts"`
}

///////////////////
// UserAccount code
///////////////////

type UserAccount struct {
	Id         string `json:"userid"`
	FullName   string `json:"fullName"`
	LastUpload string `json:"lastupload"`
}

type Users []*UserAccount

// from http://golang.org/pkg/sort/#example_Interface
func (a Users) sortByName() Users {
	name := func(a1, a2 *UserAccount) bool {
		//alpha order
		return a1.FullName > a2.FullName
	}
	By(name).sort(a)
	return a
}

func (a Users) sortByUpload() Users {
	upload := func(a1, a2 *UserAccount) bool {
		//return accounts that have a `LastUpload` first
		return a1.LastUpload > a2.LastUpload
	}
	By(upload).sort(a)
	return a
}

func (a Users) hasUploads() Users {
	var withUploads Users

	for i := range a {
		if a[i].LastUpload != "" {
			withUploads = append(withUploads, a[i])
		}
	}
	return withUploads
}

type By func(a1, a2 *UserAccount) bool

func (by By) sort(accounts Users) {
	as := &accountSorter{
		accounts: accounts,
		by:       by,
	}
	sort.Sort(as)
}

type accountSorter struct {
	accounts Users
	by       func(a1, a2 *UserAccount) bool
}

//All part of the sort interface
func (s *accountSorter) Len() int           { return len(s.accounts) }
func (s *accountSorter) Swap(i, j int)      { s.accounts[i], s.accounts[j] = s.accounts[j], s.accounts[i] }
func (s *accountSorter) Less(i, j int) bool { return s.by(s.accounts[i], s.accounts[j]) }

///////////////////
// Platform
///////////////////

var (
	prodHost    = "https://api.tidepool.io"
	stagingHost = "https://staging-api.tidepool.io"
	develHost   = "https://devel-api.tidepool.io"
	localHost   = "http://localhost:8009"
)

type platform struct {
	host   string
	client *http.Client
	*admin
}

func initPlatform(targetEnv string) *platform {

	targetEnv = strings.ToLower(targetEnv)
	fmt.Println("Run audit against:", targetEnv)

	if targetEnv == "devel" {
		return &platform{host: develHost, client: &http.Client{}, admin: &admin{}}
	} else if targetEnv == "prod" {
		return &platform{host: prodHost, client: &http.Client{}, admin: &admin{}}
	} else if targetEnv == "staging" {
		return &platform{host: stagingHost, client: &http.Client{}, admin: &admin{}}
	} else if targetEnv == "local" {
		return &platform{host: localHost, client: &http.Client{}, admin: &admin{}}
	}
	log.Fatal("No matching environment found")
	return nil
}

func (p *platform) getProfile(acc *UserAccount) error {

	urlPath := p.host + fmt.Sprintf("/metadata/%s/profile", acc.Id)

	req, err := http.NewRequest("GET", urlPath, nil)
	if err != nil {
		return err
	}
	req.Header.Add("x-tidepool-session-token", p.admin.User.Token)

	res, err := p.client.Do(req)

	if err != nil {
		return errors.New("Could attempt to find profile: " + err.Error())
	}

	switch res.StatusCode {
	case 200:
		defer res.Body.Close()
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		json.Unmarshal(data, &acc)
		return nil
	default:
		log.Printf("Failed finding public info [%d] for [%s]", res.StatusCode, acc.Id)
		return nil
	}
}

func (p *platform) getLastUpload(acc *UserAccount) error {

	urlPath := p.host + fmt.Sprintf("/query/upload/lastentry/%s", acc.Id)

	req, _ := http.NewRequest("GET", urlPath, nil)
	req.Header.Add("x-tidepool-session-token", p.admin.User.Token)

	res, err := p.client.Do(req)

	if err != nil {
		return errors.New("Could attempt to find the last upload: " + err.Error())
	}

	switch res.StatusCode {
	case 200:

		defer res.Body.Close()
		data, err := ioutil.ReadAll(res.Body)

		if err != nil {
			log.Println("Error trying to read the last upload data", err.Error())
			return nil
		}

		json.Unmarshal(data, &acc.LastUpload)
		return nil
	default:
		log.Printf("Failed finding last upload info [%d] for [%s]", res.StatusCode, acc.FullName)
		return nil
	}
}

func (p *platform) login(un, pw string) error {

	urlPath := p.host + "/auth/login"

	req, err := http.NewRequest("POST", urlPath, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(un, pw)

	res, err := p.client.Do(req)
	if err != nil {
		return errors.New(fmt.Sprint("Login request failed", err.Error()))
	}

	switch res.StatusCode {
	case 200:
		defer res.Body.Close()
		data, _ := ioutil.ReadAll(res.Body)
		json.Unmarshal(data, &p.admin.User)
		p.admin.User.Token = res.Header.Get("x-tidepool-session-token")
		return nil
	default:
		return errors.New(fmt.Sprint("Login failed", res.StatusCode))
	}
}

func (p *platform) getAdminUsers() error {

	urlPath := p.host + fmt.Sprintf("/access/groups/%s", p.admin.User.Id)

	req, err := http.NewRequest("GET", urlPath, nil)
	if err != nil {
		return err
	}
	req.Header.Add("x-tidepool-session-token", p.admin.User.Token)

	res, err := p.client.Do(req)
	if err != nil {
		return errors.New("Could attempt to find administered accounts: " + err.Error())
	}

	switch res.StatusCode {
	case 200:
		defer res.Body.Close()
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		var raw map[string]interface{}

		json.Unmarshal(data, &raw)

		for key, _ := range raw {
			p.admin.Users = append(p.admin.Users, &UserAccount{Id: string(key)})
		}
		return nil
	default:
		log.Println("Failed finding profiles", res.StatusCode)
		return nil
	}
}

///////////////////
// Admin
///////////////////

func (a *admin) loadExistingReport(path string) error {
	log.Println("loading existing report ...", path)

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

///////////////////
// App code
///////////////////

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

// and audit will find all account linked
func auditAccounts(c *cli.Context) {

	if c.String("username") == "" {
		log.Fatal("Please specify the username with the --username or -u flag.")
	}

	if c.String("env") == "" {
		log.Fatal("Please specify the environment your running against with the --env or -e flag.")
	}

	tPlatform := initPlatform(c.String("env"))

	fmt.Printf("Password: ")
	pass := gopass.GetPasswdMasked()

	err := tPlatform.login(c.String("username"), string(pass[:]))
	if err != nil {
		log.Println("Login failure")
		log.Println(err.Error())
		return
	}

	start := time.Now()

	if c.String("reportpath") != "" {
		tPlatform.admin.loadExistingReport(c.String("reportpath"))
	} else {
		log.Println("finding user accounts ...")
		err = tPlatform.getAdminUsers()
		if err != nil {
			log.Println("failed getting user accounts")
			log.Println(err.Error())
			return
		}

		log.Println("get info for each user account")
		for _, account := range tPlatform.admin.Users {
			// Fetch the account data for each account
			if err := tPlatform.getProfile(account); err != nil {
				log.Println("failed getting user account info")
				log.Println(err.Error())
				return
			}
		}
	}

	log.Println("get last upload for each user")
	for _, acc := range tPlatform.admin.Users {
		log.Println("for", acc.Id, "getting lastupload ...")
		if err := tPlatform.getLastUpload(acc); err != nil {
			if err != nil {
				log.Println("failed getting last upload for user accounts")
				log.Println(err.Error())
				return
			}
		}
		log.Println("for", acc.Id, "got", acc.LastUpload)
	}

	tPlatform.admin.Users = tPlatform.admin.Users.sortByUpload()

	jsonRpt, err := json.MarshalIndent(tPlatform.admin, "", "  ")
	if err != nil {
		log.Println("error creating audit content")
		log.Println(err.Error())
		return
	}

	reportPath := fmt.Sprintf("./audit_%s_accounts_%s_%s.txt", tPlatform.admin.User.Name, c.String("env"), time.Now().UTC().Format(time.RFC3339))

	f, err := os.Create(reportPath)
	if err != nil {
		log.Println("error creating audit file")
		log.Println(err.Error())
		return
	}
	defer f.Close()
	f.Write(jsonRpt)
	log.Println("done! here is the audit " + reportPath)

	log.Println("process took ", time.Now().Sub(start).Seconds(), "seconds")
	return

}

// and audit will find all account linked
func prepareAuditReport(c *cli.Context) {

	if c.String("reportpath") == "" {
		log.Fatal("Please specify the path to the report")
	}

	theAdmin := &admin{}
	err := theAdmin.loadExistingReport(c.String("reportpath"))
	if err != nil {
		log.Println("error loading existing report")
		log.Println(err.Error())
		return
	}

	log.Println("accounts in audit", len(theAdmin.Users))

	var toReport Users

	if c.Bool("uploads") {
		toReport = theAdmin.Users.hasUploads().sortByName()
		log.Println("accounts w uploads", len(toReport))
	} else {
		toReport = theAdmin.Users.sortByName()
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
