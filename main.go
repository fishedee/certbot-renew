package main

import (
	"encoding/json"
	"fmt"
	. "github.com/fishedee/app/log"
	. "github.com/fishedee/app/workgroup"
	. "github.com/fishedee/language"
	"io/ioutil"
	"os/exec"
	"os/user"
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
	deploy, isExist := this.data[name]
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
	cmd := exec.Command("service", "nginx", "reload")
	err := cmd.Run()
	if err != nil {
		return NewException(1, err.Error())
	}
	return nil
}

type DeployQiniu struct {
	config DeployQiniuConfig
}

type DeployQiniuConfig struct {
	AccessToken  string `json:"access_token"`
	AccessSecert string `json:"access_secert"`
	Domain       string `json:"domain"`
}

func NewDeployQiniu(config DeployQiniuConfig) (*DeployQiniu, error) {
	return &DeployQiniu{
		config: config,
	}, nil
}

func (this *DeployQiniu) Run(certName string, chainPerm string, privePem string) error {
	return NewException(1, "dos not support")
}

type Renew struct {
	config  RenewConfig
	factory *DeployFactory
}

type RenewConfig struct {
	CertName string   `json:"cert_name"`
	Deploy   []string `json:"deploy"`
}

func NewRenew(config RenewConfig, factory *DeployFactory) (*Renew, error) {
	return &Renew{
		config:  config,
		factory: factory,
	}, nil
}

func (this *Renew) GetCertName() string {
	return this.config.CertName
}

func (this *Renew) Run() error {
	certName := this.config.CertName
	cmd := exec.Command("certbot", "renew", "--cert-name", certName)
	err := cmd.Run()
	if err != nil {
		return NewException(1, err.Error())
	}

	chainPerm, err := ioutil.ReadFile("/etc/letsencrypt/live/" + certName + "/fullchain.pem")
	if err != nil {
		return NewException(1, err.Error())
	}
	privatePerm, err := ioutil.ReadFile("/etc/letsencrypt/live/" + certName + "/privkey.pem")
	if err != nil {
		return NewException(1, err.Error())
	}

	for _, deployName := range this.config.Deploy {
		deploy, err := this.factory.Get(deployName)
		if err != nil {
			return err
		}
		err = deploy.Run(certName, string(chainPerm), string(privatePerm))
		if err != nil {
			return err
		}
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
			deployQiniu, err := NewDeployQiniu(deployQiniuConfig)
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
		renew, err := NewRenew(singleRenewConfig, deployFactory)
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

func (this *Runner) Run() error {
	user, err := user.Current()
	if err != nil {
		return NewException(1, err.Error())
	}
	if user.Username != "root" {
		return NewException(1, "You should login by root")
	}
	this.log.Debug("certbot-renew is running...")
	isRunning := true
	for isRunning {
		select {
		case <-time.After(this.interval):
			for _, renew := range this.renew {
				this.log.Debug("renew cert %v begin ...", renew.GetCertName())
				err := renew.Run()
				if err != nil {
					this.log.Error("renew cert %v error %v", renew.GetCertName(), err.Error())
				}
			}
		case <-this.closeChan:
			isRunning = false
		}
	}
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
	workgroup, err := NewWorkGroup(log, WorkGroupConfig{
		CloseTimeout: time.Second * 5,
		GraceClose:   true,
	})
	workgroup.Add(runner)
	err = workgroup.Run()
	if err != nil {
		log.Error("%v", err.Error())
		return
	}
}
