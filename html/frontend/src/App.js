import React from "react";
import {
  Layout,
  message,
} from "antd";

import Banner from "./components/Banner/Banner";
import SideBar from "./components/SideBar/SideBar";
import ClientsBody from "./components/Body/ClientsBody";

import "./App.css";

const axios = require("axios");
axios.defaults.headers.post["Content-Type"] = "application/x-www-form-urlencoded";
var W3CWebSocket = require("websocket").w3cwebsocket;
var filesize = require("filesize");

message.config({
  duration: 3,
  maxCount: 5,
  rtl: true,
});


let endPoint = process.env.NODE_ENV === "development" ? "192.168.88.129:7331" : window.location.host;
let baseUrl = ["http://", endPoint].join("");
let apiUrl = [baseUrl, "/api"].join("");
let wsUrl = ["ws://", endPoint, "/notify"].join("");

class App extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      bottom: "bottomLeft",
      serversMap: null,
      serversList: [],
      isModalVisible: false,
      currentServer: null,
      connectBack: "",
      serverCreateHost: "0.0.0.0",
      serverCreatePort: Math.floor(Math.random() * 65536),
    };
    this.generateProgressStatus = this.generateProgressStatus.bind(this);
    this.handleCancel = this.handleCancel.bind(this);
    this.handleOk = this.handleOk.bind(this);
    this.selectServer = this.selectServer.bind(this);
    this.serverCreated = this.serverCreated.bind(this);
    this.setConnectBack = this.setConnectBack.bind(this);
    this.setCopied = this.setCopied.bind(this);
    this.showModal = this.showModal.bind(this);
  }

  upgradeToTermite(clientHash, target) {
    if (target !== "") {
      axios
        .get(apiUrl + "/client/" + clientHash + "/upgrade/" + target)
        .then((response) => {
          console.log(response)
        })
    } else {
      message.error("Invalid connect back termite listener address: " + target, 5);
    }
  }

  showModal() {
    this.setState({
      isModalVisible: true,
    });
  };

  handleOk(hash) {
    this.upgradeToTermite(hash, this.state.connectBack)
    this.setState({
      isModalVisible: false,
      connectBack: "",
    })
  };

  handleCancel() {
    this.setState({
      isModalVisible: false,
      connectBack: "",
    })
  };

  generateProgressStatus(prog) {
    if (prog < 0) {
      return "exception"
    } else if (prog === 0) {
      return "normal"
    } else if (prog > 0) {
      if (prog >= 100) {
        return "success"
      } else {
        return "active"
      }
    }
  }

  fetchData() {
    axios
      .get([apiUrl, "/server"].join(""))
      .then((response) => {
        if (Object.values(response.data.msg).length > 0) {
          this.setState({
            serversMap: response.data.msg.servers,
            serversList: Object.values(response.data.msg.servers),
            currentServer: Object.values(response.data.msg.servers)[0],
            distributor: response.data.msg.distributor
          });
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
    client.onerror = () => {
      // message.error("WebSocket connect failed!", 5);
    };
    client.onopen = () => {
      // message.success("WebSocket connected!", 5);
    };
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
          console.log(data);
          let onlinedClient = data.Data.Client;
          serverHash = data.Data.ServerHash;
          message.success(
            "New client connected from: " +
            onlinedClient.host +
            ":" +
            onlinedClient.port,
            5
          );

          newServersMap = this.state.serversMap;
          if (newServersMap[serverHash].encrypted) {
            newServersMap[serverHash].termite_clients[onlinedClient.hash] = onlinedClient;
          } else {
            newServersMap[serverHash].clients[onlinedClient.hash] = onlinedClient;
          }

          _this.setState({
            serversMap: newServersMap,
          });
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

          newServersMap = this.state.serversMap;
          newServersMap[serverHash].clients[clientHash].compiling_progress = cp
          if (newServersMap[serverHash].clients[clientHash].compiling_progress === 100) {
            newServersMap[serverHash].clients[clientHash].alert = "Compile sucessfully!"
          } else {
            newServersMap[serverHash].clients[clientHash].alert = "Compiling..."
          }

          _this.setState({
            serversMap: newServersMap,
          });
          break
        case COMPRESSING_TERMITE:
          let compressingProgress = data.Data;
          clientHash = compressingProgress.Client.hash;
          serverHash = compressingProgress.ServerHash;
          let p = compressingProgress.Progress

          newServersMap = this.state.serversMap;
          newServersMap[serverHash].clients[clientHash].compressing_progress = p

          if (newServersMap[serverHash].clients[clientHash].compressing_progress === 100) {
            newServersMap[serverHash].clients[clientHash].alert = "Compress successfully!"
          } else {
            newServersMap[serverHash].clients[clientHash].alert = "Compressing..."
          }

          _this.setState({
            serversMap: newServersMap,
          });
          break
        case UPLOADING_TERMITE:
          let uploadingProgress = data.Data;
          clientHash = uploadingProgress.Client.hash;
          serverHash = uploadingProgress.ServerHash;
          let bytesSent = uploadingProgress.BytesSent
          let bytesTotal = uploadingProgress.BytesTotal

          newServersMap = this.state.serversMap;
          newServersMap[serverHash].clients[clientHash].upload_progress = (bytesSent / bytesTotal) * 100;

          if (newServersMap[serverHash].clients[clientHash].upload_progress === 100) {
            newServersMap[serverHash].clients[clientHash].alert = "Upgrade successfully!"
          } else {
            newServersMap[serverHash].clients[clientHash].alert = filesize(bytesSent) + " / " + filesize(bytesTotal)
          }
          _this.setState({
            serversMap: newServersMap,
          });
          break;
        default:
          break;
      }
    };
    client.onclose = () => {
      message.error("WebSocket disconnected!", 5);
    };
  }

  serverCreated(newServer) {
    this.setState({
      serversList: [...this.state.serversList, newServer],
    });
    const newServersMap = this.state.serversMap;
    newServersMap[newServer.hash] = newServer;
    this.setState({
      serversMap: newServersMap,
      serverCreatePort: Math.floor(Math.random() * 65536),
    });
  }

  selectServer(hash) {
    this.setState({
      currentServer: this.state.serversMap[hash],
    });
  }

  setConnectBack(value) {
    this.setState({ connectBack: value });
  }

  setCopied() {
    this.setState({ copied: true })
  }

  render() {
    return (
      <Layout>
        <Banner></Banner>
        <Layout style={{ height: "100%" }}>
          <SideBar
            apiUrl={apiUrl}
            currentServer={this.state.currentServer}
            selectServer={this.selectServer}
            serverCreated={this.serverCreated}
            serverCreateHost={this.state.serverCreateHost}
            serverCreatePort={this.state.serverCreatePort}
            serversList={this.state.serversList}
            serversMap={this.state.serversMap}
          />
          <ClientsBody
            baseUrl={baseUrl}
            currentServer={this.state.currentServer}
            distributor={this.state.distributor}
            bottom={this.state.bottom}
            connectBack={this.state.connectBack}
            currentServer={this.state.currentServer}
            handleCancel={this.handleCancel}
            handleOk={this.handleOk}
            isModalVisible={this.state.isModalVisible}
            serversList={this.state.serversList}
            setConnectBack={this.setConnectBack}
            showModal={this.showModal}
            generateProgressStatus={this.generateProgressStatus}
          />
        </Layout>
      </Layout>
    );
  }
}

export default App;