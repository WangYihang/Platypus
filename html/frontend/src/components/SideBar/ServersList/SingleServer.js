import React from "react";
import { Tooltip, Badge } from "antd";
import { LockOutlined, UnlockOutlined } from '@ant-design/icons';

export default class SingleServer extends React.Component {
    generateIcon(encrypted) {
        let icon;
        if (encrypted) {
            icon = <Tooltip title={"Secure Protocol"}>
                <LockOutlined style={{ color: "green" }} />
            </Tooltip>
        } else {
            icon = <Tooltip title={"Reverse Shell Protocol"}>
                <UnlockOutlined style={{ color: "red" }} />
            </Tooltip>
        }
        return icon
    }

    generateBadge(count) {
        return <Badge
            count={count}
            overflowCount={99}
            offset={[10, 0]}
        />
    }
    render() {
        return <>
            {this.generateIcon(this.props.server.encrypted)}
            {this.props.server.host + ":" + this.props.server.port}
            {this.generateBadge(Object.keys(this.props.server.clients).length + Object.keys(this.props.server.termite_clients).length)}
        </>;
    }
}


