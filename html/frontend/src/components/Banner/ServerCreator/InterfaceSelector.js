import React from "react";
import { Select } from "antd";

const { Option } = Select;

export default class InterfaceSelector extends React.Component {
  render() {
    let interfaceMenu;
    if (this.props.currentServer === null) {
      interfaceMenu = (
        <Select
          showSearch
          style={{ width: 200 }}
          placeholder="Select an interface"
          optionFilterProp="children"
          onChange={(value) => {
            this.props.setServerCreateHost(value);
          }}
          filterOption={(input, option) =>
            option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
          }
          defaultValue={this.props.serverCreateHost}
        >
          <Option value="0.0.0.0" key="0.0.0.0">0.0.0.0</Option>
          <Option value="127.0.0.1" key="127.0.0.1">127.0.0.1</Option>
        </Select>
      );
    } else {
      interfaceMenu = (
        <Select
          showSearch
          style={{ width: 200 }}
          placeholder="Select an interface"
          optionFilterProp="children"
          onChange={(value) => {
            this.props.setServerCreateHost(value);
          }}
          filterOption={(input, option) =>
            option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
          }
        >
          <Option value="0.0.0.0" key={"0.0.0.0"}>0.0.0.0</Option>
          {Object.values(this.props.currentServer.interfaces).map((value, index) => {
            return <Option value={value} key={value}>{value}</Option>;
          })}
        </Select>
      );
    }
    return interfaceMenu;
  }
}

