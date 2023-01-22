assets:
	echo "Collecting assets files"
	go-bindata -pkg assets -o ./assets/assets.go ./assets/...

docs:
	swag init --dir cmd --output api
