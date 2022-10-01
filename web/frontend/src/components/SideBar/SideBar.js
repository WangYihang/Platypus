import React from "react";
import { Layout } from "antd";
import ServersList from "./ServersList/ServersList";

const { Sider } = Layout;

export default class SideBar extends React.Component {
  render() {
    return <Sider width={200} className="site-layout-background">
      <ServersList
        serversList={this.props.serversList}
        selectServer={this.props.selectServer}
        ToShowRbac={this.props.ToShowRbac}
        unShowRbac={this.props.unShowRbac}
      />
    </Sider>;
  }
}


