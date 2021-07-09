import React from "react";
import SingleServer from "./SingleServer";
import { Menu } from "antd";

export default class ServersList extends React.Component {
    render() {
        return             <Menu
        mode="inline"
        defaultSelectedKeys={["1"]}
        defaultOpenKeys={["sub1"]}
        style={{ height: "100%" }}
      >
        {this.props.serversList.map((value, index) => {
            return <Menu.Item
              key={value.hash}
              onClick={(item, key, keyPath, domEvent) => {
                this.props.selectServer(item.key)
              }}
            >
              <SingleServer server={value}/>
            </Menu.Item>
        })}</Menu>;
    }
}


