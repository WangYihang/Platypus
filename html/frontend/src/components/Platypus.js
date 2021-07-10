import React from "react";
import { Layout, message } from "antd";

import Banner from "./Banner/Banner";
import SideBar from "./SideBar/SideBar";
import ClientsBody from "./Body/ClientsBody";

message.config({
  duration: 3,
  maxCount: 5,
  rtl: true,
});
var W3CWebSocket = require("websocket").w3cwebsocket;
var filesize = require("filesize");

let endPoint = process.env.NODE_ENV === "development" ? "127.0.0.1:7331" : window.location.host;
let baseUrl = ["http://", endPoint].join("");
let apiUrl = [baseUrl, "/api"].join("");
let wsUrl = ["ws://", endPoint, "/notify"].join("");

const axios = require("axios");
axios.defaults.headers.post["Content-Type"] = "application/x-www-form-urlencoded";

export default class Platypus extends React.Component {
  upgradeToTermite(clientHash, target) {
    if (target !== "") {
      axios
        .get(apiUrl + "/client/" + clientHash + "/upgrade/" + target)
        .then((response) => {
        })
    } else {
      message.error("Invalid connect back termite listener address: " + target, 5);
    }
  }

  fetchData() {
    axios
      .get([apiUrl, "/server"].join(""))
      .then((response) => {
        if (Object.values(response.data.msg).length > 0) {
          this.props.setData(response.data)
        }
      })
      .catch((error) => {
        message.error("Cannot connect to API EndPoint: " + error, 5);
      });
  }

  componentDidMount() {
    let _this = this;
    _this.fetchData();

    var client = new W3CWebSocket(wsUrl);
    client.onmessage = (e) => {
      let CLIENT_CONNECTED = 0;
      let CLIENT_DUPLICATED = 1;
      let SERVER_DUPLICATED = 2;
      let COMPILING_TERMITE = 3;
      let COMPRESSING_TERMITE = 4;
      let UPLOADING_TERMITE = 5;

      let data = JSON.parse(e.data);

      let serverHash, clientHash, newServersMap

      switch (data.Type) {
        case CLIENT_CONNECTED:
          let onlinedClient = data.Data.Client;
          serverHash = data.Data.ServerHash;
          message.success(
            "New client connected from: " +
            onlinedClient.host +
            ":" +
            onlinedClient.port,
            5
          );

          newServersMap = this.props.serversMap;
          if (newServersMap[serverHash].encrypted) {
            newServersMap[serverHash].termite_clients[onlinedClient.hash] = onlinedClient;
          } else {
            newServersMap[serverHash].clients[onlinedClient.hash] = onlinedClient;
          }

          this.props.setServersMap(newServersMap)
          break;
        case CLIENT_DUPLICATED:
          let duplicatedClient = data.Data.Client;
          message.error(
            "Duplicated client connected from: " +
            duplicatedClient.host +
            ":" +
            duplicatedClient.port +
            ", connection reseted.",
            5
          );
          break;
        case SERVER_DUPLICATED:
          let duplicatedServer = data.Data;
          message.error(
            "Duplicated server: " +
            duplicatedServer.host +
            ":" +
            duplicatedServer.port,
            5
          );
          break;

        case COMPILING_TERMITE:
          let compilingProgress = data.Data;
          clientHash = compilingProgress.Client.hash;
          serverHash = compilingProgress.ServerHash;
          let cp = compilingProgress.Progress

          newServersMap = this.props.serversMap;
          newServersMap[serverHash].clients[clientHash].compiling_progress = cp
          if (newServersMap[serverHash].clients[clientHash].compiling_progress === 100) {
            newServersMap[serverHash].clients[clientHash].alert = "Compile sucessfully!"
          } else {
            newServersMap[serverHash].clients[clientHash].alert = "Compiling..."
          }

          this.props.setServersMap(newServersMap)
          break
        case COMPRESSING_TERMITE:
          let compressingProgress = data.Data;
          clientHash = compressingProgress.Client.hash;
          serverHash = compressingProgress.ServerHash;
          let p = compressingProgress.Progress

          newServersMap = this.props.serversMap;
          newServersMap[serverHash].clients[clientHash].compressing_progress = p

          if (newServersMap[serverHash].clients[clientHash].compressing_progress === 100) {
            newServersMap[serverHash].clients[clientHash].alert = "Compress successfully!"
          } else {
            newServersMap[serverHash].clients[clientHash].alert = "Compressing..."
          }

          this.props.setServersMap(newServersMap)
          break
        case UPLOADING_TERMITE:
          let uploadingProgress = data.Data;
          clientHash = uploadingProgress.Client.hash;
          serverHash = uploadingProgress.ServerHash;
          let bytesSent = uploadingProgress.BytesSent
          let bytesTotal = uploadingProgress.BytesTotal

          newServersMap = this.props.serversMap;
          newServersMap[serverHash].clients[clientHash].upload_progress = (bytesSent / bytesTotal) * 100;

          if (newServersMap[serverHash].clients[clientHash].upload_progress === 100) {
            newServersMap[serverHash].clients[clientHash].alert = "Upgrade successfully!"
          } else {
            newServersMap[serverHash].clients[clientHash].alert = filesize(bytesSent) + " / " + filesize(bytesTotal)
          }
          this.props.setServersMap(newServersMap)
          break;
        default:
          break;
      }
    };
    client.onclose = () => {
      message.error("WebSocket disconnected!", 5);
    };
  }

  render() {
    return <Layout>
      <Banner
        apiUrl={apiUrl}
        currentServer={this.props.currentServer}
        serverCreated={this.props.serverCreated}
        serverCreateHost={this.props.serverCreateHost}
        serverCreatePort={this.props.serverCreatePort}
        serverCreateEncrypted={this.props.serverCreateEncrypted}
        serversList={this.props.serversList}
        serversMap={this.props.serversMap}
        setServerCreateHost={this.props.setServerCreateHost}
        setServerCreatePort={this.props.setServerCreatePort}
        setServerCreateEncrypted={this.props.setServerCreateEncrypted}
      />
      <Layout style={{ height: "100%" }}>
        <SideBar
          apiUrl={apiUrl}
          currentServer={this.props.currentServer}
          selectServer={this.props.selectServer}
          serverCreated={this.props.serverCreated}
          serverCreateHost={this.props.serverCreateHost}
          serverCreatePort={this.props.serverCreatePort}
          serversList={this.props.serversList}
          serversMap={this.props.serversMap}
        />
        <ClientsBody
          baseUrl={baseUrl}
          currentServer={this.props.currentServer}
          distributor={this.props.distributor}
          bottom={this.props.bottom}
          connectBack={this.props.connectBack}
          handleCancel={this.props.handleCancel}
          handleOk={this.props.handleOk}
          isModalVisible={this.props.isModalVisible}
          serversList={this.props.serversList}
          setConnectBack={this.props.setConnectBack}
          showModal={this.props.showModal}
          upgradeToTermite={this.upgradeToTermite}
        />
      </Layout>
    </Layout>;
  }
}