package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strconv"

	"io/ioutil"
	"log"

	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/ProxeusApp/proxeus-core/externalnode"
	sp "github.com/SparkPost/gosparkpost"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

const serviceID = "node-mail-sender"
const defaultServiceName = "Email Sender"
const defaultJWTSecret = "my secret"
const defaultServiceUrl = "127.0.0.1"
const defaultServicePort = "8013"
const defaultAuthkey = "auth"
const defaultProxeusUrl = "http://127.0.0.1:1323"
const defaultRegisterRetryInterval = 5

type configData struct {
	EmailFrom    string
	EmailTo      string
	EmailSubject string
	EmailBody    string
	Replacement  string
}

var configPage *template.Template

type Email struct {
	From    string
	To      []string
	Subject string
	Body    string
}

type EmailSender interface {
	Send(e *Email) error
}

type configuration struct {
	proxeusUrl string
	serviceUrl string
	jwtsecret  string
	authtoken  string
	apiKey     string
}

var Config configuration

var apiKey string

func main() {
	proxeusUrl := os.Getenv("PROXEUS_INSTANCE_URL")
	if len(proxeusUrl) == 0 {
		proxeusUrl = defaultProxeusUrl
	}
	servicePort := os.Getenv("SERVICE_PORT")
	if len(servicePort) == 0 {
		servicePort = defaultServicePort
	}
	serviceUrl := os.Getenv("SERVICE_URL")
	if len(serviceUrl) == 0 {
		serviceUrl = "http://localhost:" + servicePort
	}
	jwtsecret := os.Getenv("SERVICE_SECRET")
	if len(jwtsecret) == 0 {
		jwtsecret = defaultJWTSecret
	}
	serviceName := os.Getenv("SERVICE_NAME")
	if len(serviceName) == 0 {
		serviceName = defaultServiceName
	}
	registerRetryInterval_input := os.Getenv("REGISTER_RETRY_INTERVAL")
	registerRetryInterval := defaultRegisterRetryInterval
	if len(registerRetryInterval_input) >= 0 {
		registerRetryInterval_parsed, err := strconv.Atoi(registerRetryInterval_input)
		if err == nil {
			registerRetryInterval = registerRetryInterval_parsed
		}
	}
	Config = configuration{proxeusUrl: proxeusUrl, serviceUrl: serviceUrl, jwtsecret: jwtsecret, authtoken: defaultAuthkey}
	fmt.Println()
	fmt.Println("#######################################################")
	fmt.Println("# STARTING NODE - " + serviceName)
	fmt.Println("# listing on " + serviceUrl)
	fmt.Println("# connecting to " + proxeusUrl)
	fmt.Println("#######################################################")
	fmt.Println()

	apiKey = os.Getenv("PROXEUS_SPARKPOST_API_KEY")
	if len(apiKey) == 0 {
		log.Panic("PROXEUS_SPARKPOST_API_KEY needs to be configured")
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.GET("/health", externalnode.Health)
	{
		g := e.Group("/node/:id")
		conf := middleware.DefaultJWTConfig
		conf.SigningKey = []byte(jwtsecret)
		conf.TokenLookup = "query:" + defaultAuthkey
		g.Use(middleware.JWTWithConfig(conf))

		g.POST("/next", next)
		g.GET("/config", config)
		g.POST("/config", setConfig)
	}

	//External Node Specific Initialization
	parseTemplates()

	//Common External Node registration
	externalnode.Register(proxeusUrl, serviceName, serviceUrl, jwtsecret, "Send Emails", registerRetryInterval)
	err := e.Start("0.0.0.0:" + servicePort)
	if err != nil {
		log.Printf("[%s][run] err: %s", serviceName, err.Error())
	}
}

func next(c echo.Context) error {
	body, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		return err
	}

	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return c.JSON(http.StatusOK, response)
	}

	log.Println("Node mail sender called")
	mailSender, err := NewSparkPostEmailSender(apiKey, "")
	if err != nil {
		log.Println("Can't initialize sparkPostEmailSender: " + err.Error())
		return c.JSON(http.StatusOK, response)
	}

	config := getConfig(c)

	err = mailSender.Send(&Email{
		From:    config.EmailFrom,
		To:      []string{config.EmailTo},
		Subject: config.EmailSubject,
		Body: fmt.Sprintf(
			config.EmailBody, response[config.Replacement],
		),
	})
	if err != nil {
		return c.JSON(http.StatusOK, response)
	}
	log.Println("MailSender node's email sent")

	return c.JSON(http.StatusOK, response)
}

func config(c echo.Context) error {
	id, err := externalnode.NodeID(c)
	if err != nil {
		return err
	}
	conf := getConfig(c)
	var buf bytes.Buffer
	err = configPage.Execute(&buf, map[string]string{
		"Id":           id,
		"AuthToken":    c.QueryParam(Config.authtoken),
		"EmailFrom":    conf.EmailFrom,
		"EmailTo":      conf.EmailTo,
		"EmailSubject": conf.EmailSubject,
		"EmailBody":    conf.EmailBody,
		"Replacement":  conf.Replacement,
	})
	if err != nil {
		return err
	}
	return c.Stream(http.StatusOK, "text/html", &buf)
}

func setConfig(c echo.Context) error {
	conf := &configData{
		EmailFrom:    strings.TrimSpace(c.FormValue("EmailFrom")),
		EmailTo:      strings.TrimSpace(c.FormValue("EmailTo")),
		EmailSubject: strings.TrimSpace(c.FormValue("EmailSubject")),
		EmailBody:    strings.TrimSpace(c.FormValue("EmailBody")),
		Replacement:  strings.TrimSpace(c.FormValue("Replacement")),
	}
	if conf.EmailFrom == "" || conf.EmailTo == "" || conf.EmailSubject == "" || conf.EmailBody == "" {
		return c.String(http.StatusBadRequest, "empty fields")
	}

	err := externalnode.SetStoredConfig(c, Config.proxeusUrl, conf)
	if err != nil {
		return err
	}
	return config(c)
}

func getConfig(c echo.Context) *configData {
	jsonBody, err := externalnode.GetStoredConfig(c, Config.proxeusUrl)
	if err != nil {
		return &configData{
			EmailFrom:    "no-reply@proxeus.com",
			EmailTo:      "no-reply@proxeus.com",
			EmailSubject: "Subject",
			EmailBody:    "Hey, this has been sent from the flow on workflow. CHF/XES: %s",
			Replacement:  "CHFXES",
		}
	}

	config := configData{}
	if err := json.Unmarshal(jsonBody, &config); err != nil {
		fmt.Println(err)
		return nil
	}
	return &config
}

func parseTemplates() {
	var err error
	configPage, err = template.New("").Parse(configHTML)
	if err != nil {
		panic(err.Error())
	}
}

const configHTML = `
<!DOCTYPE html>
<html>
<body>
<form action="/node/{{.Id}}/config?auth={{.AuthToken}}" method="post">
Email from: <input type="text" size="30" name="EmailFrom" value="{{.EmailFrom}}">
Email to: <input type="text" size="30" name="EmailTo" value="{{.EmailTo}}">
Email subject: <input type="text" size="30" name="EmailSubject" value="{{.EmailSubject}}">
Email body: <br>
<textarea id="w3mission" rows="8" cols="80">
{{.EmailBody}}
</textarea>
Replacement variable: <input type="text" size="30" name="Replacement" value="{{.Replacement}}">
<input type="submit" value="Submit">
</form>
</body>
</html>
`

var TestMode bool

type sparkPostEmailSender struct {
	client           *sp.Client
	defaultEmailFrom string
}

func NewSparkPostEmailSender(apiKey, defaultEmailFrom string) (EmailSender, error) {
	cfg := &sp.Config{
		BaseUrl:    "https://api.sparkpost.com",
		ApiKey:     apiKey,
		ApiVersion: 1,
	}

	client := &sp.Client{}
	err := client.Init(cfg)
	if err != nil {
		return nil, err
	}
	return &sparkPostEmailSender{client: client, defaultEmailFrom: defaultEmailFrom}, nil
}

func (me *sparkPostEmailSender) Send(e *Email) error {
	emailFrom := e.From
	if len(emailFrom) == 0 {
		emailFrom = me.defaultEmailFrom
	}
	if len(emailFrom) == 0 {
		return errors.New("the From attribute has to be populated to send an email")
	}
	content := sp.Content{
		From:    emailFrom,
		Subject: e.Subject,
	}
	if body := strings.TrimLeftFunc(e.Body, unicode.IsSpace); strings.HasPrefix(body, "<") {
		content.HTML = e.Body
	} else {
		content.Text = e.Body
	}
	tx := &sp.Transmission{
		Recipients: e.To,
		Content:    content,
	}
	if TestMode {
		return nil
	}
	_, _, err := me.client.Send(tx)
	return err
}
