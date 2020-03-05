package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"io/ioutil"
	"log"

	"net/http"
	"os"
	"strconv"
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
	}

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

	err = mailSender.Send(&Email{
		From:    "no-reply@proxeus.com",
		To:      []string{"no-reply@proxeus.com"},
		Subject: "Workflow example connector",
		Body: fmt.Sprintf(
			"Hey, this has been sent from the flow on workflow. CHF/XES: %s", response["CHFXES"],
		),
	})
	if err != nil {
		return c.JSON(http.StatusOK, response)
	}
	log.Println("MailSender node's email sent")

	return c.JSON(http.StatusOK, response)
}

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
