import React, { Component } from 'react'





export default class RightTable extends Component {
    state = {
        isChecked : false,
        rightList :[],
        
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
                            <span>{hasCreate? rightitem.info :rightitem.role}</span>
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

