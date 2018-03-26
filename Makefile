run:
	go build 
	sudo ./certbot-renew
release:
	go build
	sudo nohup ./certbot-renew & 2>&1
