import React from "react";
import { Layout } from "antd";
import ServerCreator from "./ServerCreator/SeverCreator";
import ServersList from "./ServersList/ServersList";

const { Sider } = Layout;

export default class SideBar extends React.Component {
  render() {
    return <Sider width={200} className="site-layout-background">
      <ServerCreator
        apiUrl={this.props.apiUrl}
        currentServer={this.props.currentServer}
        serverCreated={this.props.serverCreated}
        serverCreateHost={this.props.serverCreateHost}
        serverCreatePort={this.props.serverCreatePort}
        serversList={this.props.serversList}
        serversMap={this.props.serversMap}
      />

      <ServersList
        serversList={this.props.serversList}
        selectServer={this.props.selectServer}
      />
    </Sider>;
  }
}


