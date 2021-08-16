import React from "react";
import PortSelector from "./PortSelector";
import InterfaceSelector from "./InterfaceSelector";
import CreateServerButton from "./CreateServerButton";
import { Switch } from 'antd';

export default class ServerCreator extends React.Component {
    render() {
        return <>
            <PortSelector
                serverCreatePort={this.props.serverCreatePort}
                setServerCreatePort={this.props.setServerCreatePort}
            />
            <InterfaceSelector
                currentServer={this.props.currentServer}
                serverCreateHost={this.props.serverCreateHost}
                serverCreatePort={this.props.serverCreatePort}
                setServerCreateHost={this.props.setServerCreateHost}
            />
            <Switch checkedChildren="Encrypted" unCheckedChildren="Plained" defaultChecked onChange={this.props.setServerCreateEncrypted} />
            <CreateServerButton
                apiUrl={this.props.apiUrl}
                serverCreated={this.props.serverCreated}
                serverCreateHost={this.props.serverCreateHost}
                serverCreatePort={this.props.serverCreatePort}
                serverCreateEncrypted={this.props.serverCreateEncrypted}
                serversList={this.props.serversList}
                serversMap={this.props.serversMap}
                setServerCreateHost={this.props.setServerCreateHost}
                setServerCreatePort={this.props.setServerCreatePort}
            />
        </>;
    }
}
