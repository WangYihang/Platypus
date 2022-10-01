import React, { Component } from 'react'
import Access from "../Access";





export default class RightTable extends Component {
    state = {
        isChecked : false,
        rightList :[],
        
    }
    showOS=(data) => {
        switch (data) {
            case 1:
                return "Linux"
            case 2:
                return "Windows";
            case 3:
                return "SunOS";
            case 4:
                return "MacOS";
            case 5:
                return "FreeBSD";
            default:
                return "Unknown Operating System";
        }
    }

    funx=(newList)=>{
        let filterItem = newList.filter(rightitem=>{
            return rightitem.get
        })
        
        if (filterItem.length===0){
            this.setState({isChecked:false})
        }
        if (filterItem.length===newList.length){
            this.setState({isChecked:true})
        }else{
            this.setState({isChecked:false})
        }
    }

    changeSingleRow=(rightitemName)=>{
        if(this.props.hasCreate){
            let newList 
            newList = this.state.rightList.map(rightitem=>{
                        if (rightitem.hash===rightitemName){
                            rightitem.get = !rightitem.get
                        }
                        return rightitem
            })
            this.setState({rightList:newList})
            let filterItem = newList.filter(rightitem=>{
                return rightitem.get
            })
            
            if (filterItem.length===0){
                this.setState({isChecked:false})
            }
            if (filterItem.length===newList.length){
                this.setState({isChecked:true})
            }else{
                this.setState({isChecked:false})
            }
        }else{
            let newList 
            newList = this.state.rightList.map(rightitem=>{
                        if (rightitem.role===rightitemName){
                            rightitem.get = !rightitem.get
                        }
                        return rightitem
            })
            this.setState({rightList:newList})
            let filterItem = newList.filter(rightitem=>{
                return rightitem.get
            })
            
            if (filterItem.length===0){
                this.setState({isChecked:false})
            }
            if (filterItem.length===newList.length){
                this.setState({isChecked:true})
            }else{
                this.setState({isChecked:false})
            }
        }
        

    }
    

   selectAll=(e)=>{
    this.setState({isChecked:e.target.checked})
    this.state.rightList.map(rightitem=>rightitem.get=e.target.checked)
   }

   componentWillReceiveProps(nextProps, nextContext){
    this.setState({rightList:nextProps.rightList})
    this.funx(nextProps.rightList)
   }
  render() {
    const {hasCreate}= this.props
    return (
        <div className="rightTableDiv">
        <table className="table">
            {
                this.state.rightList.map(rightitem=>{
                    return (
                    <tr>
                        <td className="row">
                            <input type="checkbox"  checked={rightitem.get} onChange={()=>{this.changeSingleRow(hasCreate? rightitem.hash:rightitem.role)}} />
                            {hasCreate? <Access hash={rightitem.hash} address={rightitem.address} os={this.showOS(rightitem.os)} user={rightitem.user} timestamp={rightitem.timestamp}/> :<span>{rightitem.role}</span>}
                        </td>
                    </tr>
                    )
                })
            }
            <tr>
                <td className="row">
                    <input   type="checkbox"  checked={this.state.isChecked} onChange={(e)=>{this.selectAll(e)}}/>
                    <span>全选/全不选</span>
                </td>
            </tr>
        </table>
    </div>
    )
  }
}

