import React from "react";
import { Layout } from "antd";
import ServerCreator from "./ServerCreator/SeverCreator";

export default class Banner extends React.Component {
    render() {
        return <Layout.Header className="header">
            <div className="logo" />
            <h1>
                <a href="https://github.com/WangYihang/Platypus" rel="noreferrer noopener" target="_blank">Platypus</a>
                <ServerCreator
                    apiUrl={this.props.apiUrl}
                    currentServer={this.props.currentServer}
                    serverCreated={this.props.serverCreated}
                    serverCreateHost={this.props.serverCreateHost}
                    serverCreatePort={this.props.serverCreatePort}
                    serversList={this.props.serversList}
                    serversMap={this.props.serversMap}
                    setServerCreateHost={this.props.setServerCreateHost}
                    setServerCreatePort={this.props.setServerCreatePort}
                />
            </h1>
        </Layout.Header>
    }
}