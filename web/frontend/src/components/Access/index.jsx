import React, {Component} from 'react';
const moment = require("moment");
export default class Access extends Component {
    render() {
        return (
            <span title={this.props.hash}><font color="blue">Address:{this.props.address}</font>   <font color="red">OS:{this.props.os}</font>   <font color="aqua">Username:{this.props.user}</font>   <font color="black">Online Time:{moment(this.props.timestamp).fromNow()}</font></span>
        );
    }
}
