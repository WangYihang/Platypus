import axios from 'axios'
import React, { Component } from 'react'

export default class SaveButton extends Component {

  postSaveData=()=>{
    if (this.props.hasCreate){
      axios
      .post([this.props.rbacUrl, "/roleAccesses"].join(""),{"rolename":this.props.leftName,"accesses":this.props.rightList})
      .then((response)=>{
        this.props.refreshFunc()
      })
      .catch(
        //
      )
    }else{
      axios
      .post([this.props.rbacUrl, "/userRoles"].join(""),{"username":this.props.leftName,"roles":this.props.rightList})
      .then((response)=>{
        this.props.refreshFunc()
      })
      .catch(
        //
      )
    }
    

  }
  
  render() {
    return (
        <input className="saveButton" type="submit" value="保存" onClick={this.postSaveData}/>

    )
  }
}
