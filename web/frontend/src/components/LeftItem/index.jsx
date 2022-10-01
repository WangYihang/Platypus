import React, { Component } from 'react'
import axios from 'axios';

export default class LeftItem extends Component {

  postData=(leftitem,hasCreate)=>{
    const {getrightlist,getLeftName} = this.props
    getLeftName(leftitem)
    if (hasCreate){
        console.log("leftitem: ", leftitem)
      axios
          .get([this.props.rbacUrl, "/role/", leftitem].join(""))
          .then((response) => {
            getrightlist(response.data.msg)
          })
          .catch((error) => {

            // message.error("Cannot connect to API EndPoint: " + error, 5);
          });
    }
    else{
      axios
          .get([this.props.rbacUrl, "/user/", leftitem].join(""))
          .then((response) => {
            getrightlist(response.data.msg)
          })
          .catch((error) => {

            // message.error("Cannot connect to API EndPoint: " + error, 5);
          });
    }

  }


  render() {

    const {leftitem,hasCreate} = this.props
    return (
        <tr >
          <td className="row">
            <input type="radio" name="onlyOne" onClick={()=>this.postData(leftitem,hasCreate)}/>
            <label ><span>{leftitem}</span></label>
          </td>
        </tr>
    )
  }
}
