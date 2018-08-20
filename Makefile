run:
	go build 
	sudo ./certbot-renew
stop:
	sudo pkill certbot-renew
release:
	go build
	sudo nohup ./certbot-renew & 2>&1
