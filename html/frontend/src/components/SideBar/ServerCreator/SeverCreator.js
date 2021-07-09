import React from "react";
import PortSelector from "./PortSelector";
import InterfaceSelector from "./InterfaceSelector";
import CreateServerButton from "./CreateServerButton";

export default class ServerCreator extends React.Component {
    render() {
        return <>
            <PortSelector
                serverCreatePort={this.props.serverCreatePort}
            />

            <InterfaceSelector
                currentServer={this.props.currentServer}
                serverCreateHost={this.props.serverCreateHost}
            />
            <CreateServerButton
                serverCreateHost={this.props.serverCreateHost}
                serverCreatePort={this.props.serverCreatePort}
                serversList={this.props.serversList}
                serversMap={this.props.serversMap}
                serverCreated={this.props.serverCreated}
                apiUrl={this.props.apiUrl}
            />
        </>;
    }
}
