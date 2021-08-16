import React from "react";
import { Modal, Button, Select, Input } from "antd";

const { Option } = Select;

export default class UpgradeToTermite extends React.Component {
    render() {
        let upgradeButton;
        if (this.props.line.CurrentProcessKey === undefined) {
            upgradeButton = <Button disabled={false} onClick={() => { this.props.showModal() }}>Upgrade</Button>
        } else {
            upgradeButton = ""
        }

        return (
            <>
                <Button>
                    <a
                        href={this.props.baseUrl + "/shell/?" + this.props.line.hash}
                        target={"_blank"}
                        rel={"noreferrer noopener"}
                    >
                        Shell
                    </a>
                </Button>
                {upgradeButton}
                <Modal title="Basic Modal" visible={this.props.isModalVisible} onOk={() => {
                    this.props.upgradeToTermite(this.props.line.hash, this.props.connectBack)
                    this.props.handleOk(this.props.line.hash)
                }} onCancel={() => this.props.handleCancel()}>
                    Select Termite Listeners:
                    <Select
                        showSearch
                        style={{ width: 200 }}
                        placeholder="Select an termite listener"
                        optionFilterProp="children"
                        onChange={(value) => {
                            this.props.setConnectBack(value)
                        }}
                        filterOption={(input, option) =>
                            option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
                        }
                        defaultValue={this.props.connectBack}
                    >
                        {this.props.serversList.map((entry) => {
                            if (entry.encrypted) {
                                let interfaces = [...entry.interfaces];
                                if (entry.public_ip) {
                                    interfaces.unshift(entry.public_ip)
                                }
                                return interfaces.map((ifaddr) => {
                                    let v = ifaddr + ":" + entry.port
                                    return <Option value={v}>{v}</Option>
                                })
                            }
                            return ""
                        })}
                    </Select>
                    Input Termite Listeners Manually:
                    <Input placeholder="1.3.3.7:13337" value={this.props.connectBack} onChange={(e) => {
                        this.props.setConnectBack(e.target.value)
                    }} />
                </Modal>
            </>
        );
    }
}



