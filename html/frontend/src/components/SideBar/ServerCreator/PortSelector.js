import React from "react";
import { InputNumber } from "antd";

export default class PortSelector extends React.Component {
  render() {
    return <InputNumber
      min={1}
      max={65565}
      defaultValue={this.props.serverCreatePort}
      value={this.props.serverCreatePort}
      onChange={(data) => {
        this.setState({
          serverCreatePort: parseInt(data),
        });
      }}
    />;
  }
}

