package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	. "github.com/fishedee/app/log"
	. "github.com/fishedee/language"
	"io/ioutil"
	"net/http"
	"os/exec"
	"os/user"
	"qiniupkg.com/api.v7/auth/qbox"
	"time"
)

type Duration struct {
	time.Duration
}

func (this Duration) MarshalJSON() ([]byte, error) {
	return []byte(this.Duration.String()), nil
}

func (this *Duration) UnmarshalJSON(data []byte) error {
	var err error
	if len(data) <= 3 {
		return fmt.Errorf("less than length 3")
	}
	this.Duration, err = time.ParseDuration(string(data[1 : len(data)-1]))
	return err
}

type Deploy interface {
	Run(certName string, chainPerm string, privePem string) error
}

type DeployFactory struct {
	data map[string]Deploy
}

func NewDeployFactory() (*DeployFactory, error) {
	return &DeployFactory{
		data: map[string]Deploy{},
	}, nil
}

func (this *DeployFactory) Add(name string, deploy Deploy) error {
	_, isExist := this.data[name]
	if isExist == true {
		return NewException(1, "get deploy "+name+" dos exist")
	}
	this.data[name] = deploy
	return nil
}

func (this *DeployFactory) Get(name string) (Deploy, error) {
	deploy, isExist := this.data[name]
	if isExist == false {
		return nil, NewException(1, "get deploy "+name+" dos not exist")
	}
	return deploy, nil
}

type DeployNginx struct {
}

type DeployNginxConfig struct {
	Address string `json:"address"`
}

func NewDeployNginx(config DeployNginxConfig) (*DeployNginx, error) {
	if config.Address != "127.0.0.1" &&
		config.Address != "localhost" {
		return nil, NewException(1, "deploy nginx only support localhost")
	}
	return &DeployNginx{}, nil
}

func (this *DeployNginx) Run(certName string, chainPerm string, privePem string) error {
	cmd := exec.Command("service", "nginx", "restart")
	err := cmd.Run()
	if err != nil {
		return NewException(1, err.Error())
	}
	return nil
}

type DeployQiniu struct {
	log    Log
	config DeployQiniuConfig
}

type DeployQiniuConfig struct {
	AccessToken  string   `json:"access_token"`
	AccessSecert string   `json:"access_secert"`
	Domains      []string `json:"domains"`
}

func NewDeployQiniu(log Log, config DeployQiniuConfig) (*DeployQiniu, error) {
	return &DeployQiniu{
		log:    log,
		config: config,
	}, nil
}

func (this *DeployQiniu) GetCertList(client *http.Client) (map[string]interface{}, error) {
	resp, err := client.Get("https://api.qiniu.com/sslcert")
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	jsonData := map[string]interface{}{}
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	return jsonData, nil
}

func (this *DeployQiniu) GetSingleCert(client *http.Client, certId string) (map[string]interface{}, error) {
	resp, err := client.Get("https://api.qiniu.com/sslcert/" + certId)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	jsonData := map[string]interface{}{}
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	return jsonData, nil
}

func (this *DeployQiniu) AddCert(client *http.Client, param interface{}) (map[string]interface{}, error) {
	dataJson, err := json.Marshal(param)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	req, err := http.NewRequest("POST", "https://api.qiniu.com/sslcert", bytes.NewReader(dataJson))
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	jsonData := map[string]interface{}{}
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	return jsonData, nil
}

func (this *DeployQiniu) ModDomainCert(client *http.Client, domain string, param interface{}) (map[string]interface{}, error) {
	dataJson, err := json.Marshal(param)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	req, err := http.NewRequest("PUT", "https://api.qiniu.com/domain/"+domain+"/httpsconf", bytes.NewReader(dataJson))
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	jsonData := map[string]interface{}{}
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	return jsonData, nil
}

func (this *DeployQiniu) Run(certName string, chainPerm string, privePem string) error {
	client := qbox.NewClient(&qbox.Mac{
		AccessKey: this.config.AccessToken,
		SecretKey: []byte(this.config.AccessSecert),
	}, &http.Transport{})

	//查看证书列表
	certList, err := this.GetCertList(client)
	if err != nil {
		return err
	}
	isExist := false

	for _, cert := range certList["certs"].([]interface{}) {
		singleCert := cert.(map[string]interface{})
		certId := singleCert["certid"].(string)
		certInfo, err := this.GetSingleCert(client, certId)
		if err != nil {
			return err
		}
		certInfo = certInfo["cert"].(map[string]interface{})
		pri := certInfo["pri"].(string)
		ca := certInfo["ca"].(string)
		if chainPerm == ca && privePem == pri {
			isExist = true
			break
		}
	}
	if isExist == true {
		return nil
	}

	//上传新证书
	name := "qiniu_" + time.Now().Format("20060102150405")
	addCertResult, err := this.AddCert(client, map[string]interface{}{
		"Name": name,
		"Pri":  privePem,
		"Ca":   chainPerm,
	})
	if err != nil {
		return err
	}
	certId := addCertResult["certID"].(string)

	this.log.Debug("qiniu add cert id:%v,name:%v", certId, name)
	//更新域名证书
	for _, domain := range this.config.Domains {
		_, err = this.ModDomainCert(client, domain, map[string]interface{}{
			"certid":     certId,
			"forceHttps": true,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

type Renew struct {
	config  RenewConfig
	factory *DeployFactory
	log     Log
}

type RenewConfig struct {
	CertName string   `json:"cert_name"`
	Deploy   []string `json:"deploy"`
}

func NewRenew(log Log, config RenewConfig, factory *DeployFactory) (*Renew, error) {
	return &Renew{
		log:     log,
		config:  config,
		factory: factory,
	}, nil
}

func (this *Renew) GetCertName() string {
	return this.config.CertName
}

func (this *Renew) Run() error {
	certName := this.config.CertName

	chainPerm, err := ioutil.ReadFile("/etc/nginx/ssl/cert.pem")
	if err != nil {
		return NewException(1, err.Error())
	}
	privatePerm, err := ioutil.ReadFile("/etc/nginx/ssl/key.pem")
	if err != nil {
		return NewException(1, err.Error())
	}

	for _, deployName := range this.config.Deploy {
		this.log.Debug("deploy %v begin...", deployName)
		deploy, err := this.factory.Get(deployName)
		if err != nil {
			return err
		}
		err = deploy.Run(certName, string(chainPerm[0:len(chainPerm)-1]), string(privatePerm[0:len(privatePerm)-1]))
		if err != nil {
			return err
		}
		this.log.Debug("deploy %v end...", deployName)
	}
	return nil
}

type Config struct {
	Interval Duration          `json:"interval"`
	Deploy   []json.RawMessage `json:"deploy"`
	Renew    []json.RawMessage `json:"renew"`
}

type Runner struct {
	log       Log
	interval  time.Duration
	renew     []*Renew
	closeChan chan bool
}

func NewRunner(log Log, filename string) (*Runner, error) {
	config := Config{}
	configData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	err = json.Unmarshal(configData, &config)
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	//读取interval
	interval := config.Interval.Duration

	//读取deploy
	deployFactory, err := NewDeployFactory()
	if err != nil {
		return nil, NewException(1, err.Error())
	}
	for _, deployConfig := range config.Deploy {
		var driver struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		err := json.Unmarshal(deployConfig, &driver)
		if err != nil {
			return nil, NewException(1, err.Error())
		}
		if driver.Type == "nginx" {
			var deployNginxConfig DeployNginxConfig
			err := json.Unmarshal(deployConfig, &deployNginxConfig)
			if err != nil {
				return nil, NewException(1, err.Error())
			}
			deployNginx, err := NewDeployNginx(deployNginxConfig)
			if err != nil {
				return nil, err
			}
			err = deployFactory.Add(driver.Name, deployNginx)
			if err != nil {
				return nil, err
			}
		} else if driver.Type == "qiniu" {
			var deployQiniuConfig DeployQiniuConfig
			err := json.Unmarshal(deployConfig, &deployQiniuConfig)
			if err != nil {
				return nil, NewException(1, err.Error())
			}
			deployQiniu, err := NewDeployQiniu(log, deployQiniuConfig)
			if err != nil {
				return nil, err
			}
			err = deployFactory.Add(driver.Name, deployQiniu)
			if err != nil {
				return nil, err
			}
		}
	}

	//读取renew
	renews := []*Renew{}
	for _, renewConfig := range config.Renew {
		var singleRenewConfig RenewConfig
		err := json.Unmarshal(renewConfig, &singleRenewConfig)
		if err != nil {
			return nil, NewException(1, err.Error())
		}
		renew, err := NewRenew(log, singleRenewConfig, deployFactory)
		if err != nil {
			return nil, err
		}
		renews = append(renews, renew)
	}

	return &Runner{
		log:       log,
		interval:  interval,
		renew:     renews,
		closeChan: make(chan bool),
	}, nil
}

func (this *Runner) runSingle() {
	for _, renew := range this.renew {
		this.log.Debug("renew cert %v begin ...", renew.GetCertName())
		err := renew.Run()
		if err != nil {
			this.log.Error("renew cert %v error %v", renew.GetCertName(), err.Error())
		} else {
			this.log.Debug("renew cert %v finish", renew.GetCertName())
		}
	}
}
func (this *Runner) Run() error {
	user, err := user.Current()
	if err != nil {
		return NewException(1, err.Error())
	}
	this.log.Debug("certbot-renew is running... %v",user)
	this.runSingle()
	return nil
}

func (this *Runner) Close() {
	close(this.closeChan)
}

func main() {
	log, err := NewLog(LogConfig{
		Driver: "console",
	})
	if err != nil {
		fmt.Printf("%v\n", err.Error())
		return
	}
	runner, err := NewRunner(log, "./conf.json")
	if err != nil {
		log.Error("%v", err.Error())
		return
	}
	err = runner.Run()
	if err != nil{
		log.Error("%v",err.Error())
	}
	fmt.Println("success")
}
