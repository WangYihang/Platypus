import React from "react";
import { Alert, Progress, Table, Tag, Tooltip } from "antd";

import UpgradeToTermite from "../../Modal/UpgradeToTermite/UpgradeToTermite";

const moment = require("moment");
export default class ClientTable extends React.Component {
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
            return <UpgradeToTermite
              baseUrl={this.props.baseUrl}
              line={line}
              isModalVisible={this.props.isModalVisible}
              connectBack={this.props.connectBack}
              serversList={this.props.serversList}
              setConnectBack={this.props.setConnectBack}
              showModal={this.props.showModal}
              handleCancel={this.props.handleCancel}
              handleOk={this.props.handleOk}
            />
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
              <Progress percent={line.compiling_progress} size="small" status={this.props.generateProgressStatus(line.compiling_progress)} />
              <Progress percent={line.compressing_progress} size="small" status={this.props.generateProgressStatus(line.compressing_progress)} />
              <Progress percent={Math.round(line.upload_progress)} size="small" status={this.props.generateProgressStatus(line.upload_progress)} />
            </>
          },
        }
      ];
  
  
      let dataSource;
      let table;
      if (this.props.currentServer) {
        if (this.props.currentServer.encrypted) {
          dataSource = Object.values(this.props.currentServer.termite_clients)
          table = <Table
            columns={columns.slice(0, columns.length - 1)}
            pagination={{ position: [this.props.bottom] }}
            dataSource={dataSource}
          />
        } else {
          dataSource = Object.values(this.props.currentServer.clients)
          table = <Table
            columns={columns}
            pagination={{ position: [this.props.bottom] }}
            dataSource={dataSource}
          />
        }
      } else {
        table = <Table
            columns={columns}
            pagination={{ position: [this.props.bottom] }}
        />
      }
      
      return table;
    }
}
