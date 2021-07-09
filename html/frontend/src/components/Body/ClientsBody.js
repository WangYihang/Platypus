import React from "react";
import { Divider, Layout } from "antd";
import Hint from "./Hint/Hint";
import ClientTable from "./ClientTable/ClientTable";

const { Content } = Layout;

export default class ClientsBody extends React.Component {
    render() {
        return <>
            <Layout style={{ padding: "0 24px 24px" }}>
                <Content style={{ margin: "0 0" }}>
                    <Hint
                        currentServer={this.props.currentServer}
                        distributor={this.props.distributor}
                    />
                    <Divider orientation="left"></Divider>
                    <ClientTable
                        baseUrl={this.props.baseUrl}
                        bottom={this.props.bottom}
                        connectBack={this.props.connectBack}
                        currentServer={this.props.currentServer}
                        handleCancel={this.props.handleCancel}
                        handleOk={this.props.handleOk}
                        isModalVisible={this.props.isModalVisible}
                        serversList={this.props.serversList}
                        setConnectBack={this.props.setConnectBack}
                        showModal={this.props.showModal}
                        upgradeToTermite={this.props.upgradeToTermite}
                    />
                </Content>
            </Layout>
        </>;
    }
}

