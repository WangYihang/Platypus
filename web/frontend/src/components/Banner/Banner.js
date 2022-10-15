import React from "react";
import { Layout } from "antd";
import ServerCreator from "./ServerCreator/SeverCreator";
import { Row, Col } from 'antd';
export default class Banner extends React.Component {
    render() {
        return <Layout.Header className="header">
            <h1>
                <Row justify="end">
                    <Col span={6}>
                        {/* <div className="logo" /> */}
                        <a href="https://github.com/WangYihang/Platypus" rel="noreferrer noopener" target="_blank">Platypus</a>
                    </Col>
                    <Col span={12} offset={6}>
                        <ServerCreator
                            apiUrl={this.props.apiUrl}
                            currentServer={this.props.currentServer}
                            serverCreated={this.props.serverCreated}
                            serverCreateHost={this.props.serverCreateHost}
                            serverCreatePort={this.props.serverCreatePort}
                            serverCreateEncrypted={this.props.serverCreateEncrypted}
                            serversList={this.props.serversList}
                            serversMap={this.props.serversMap}
                            setServerCreateHost={this.props.setServerCreateHost}
                            setServerCreatePort={this.props.setServerCreatePort}
                            setServerCreateEncrypted={this.props.setServerCreateEncrypted}
                        />
                        <span>   </span>
                        <button className="ant-btn ant-btn-primary"  style={{borderRadius:25}} title="点击退出登录" onClick={this.props.logOut}>登出</button>
                    </Col>
                    <Col>

                    </Col>
                </Row>
            </h1>
        </Layout.Header>
    }
}