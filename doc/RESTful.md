# RESTful API

Platypus supports RESTful API to perform common operations. Almost all 
operations you can use in cli mode also can be accomplished via RESTful API.
Here are all supported RESTful APIs. You can use them to build your frontend 
applications.

* `GET /server` List all listening servers and its' related clients
* `GET /server/:hash` List a specific server
* `GET /server/:hash/client` List all clients of a specific server
* `POST /server` Create a reverse shell server
  * `host` The host you want the server to listen on
  * `port` The port you want the server to listen on
* `DELETE /server/:hash` Stop a reverse shell server
* `GET /client` List all online clients
* `GET /client/:hash` List a specific client
* `DELETE /client/:hash` Delete a reverse shell client
* `POST /client/:hash` Execute a system command on the specific client
  * `cmd` The command you want the client to execute

