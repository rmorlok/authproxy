import{j as n}from"./jsx-runtime-BjG_zV1W.js";import{C as N,a as z,M as H,r as L,b as A,S as a,c as y}from"./index-BZxc4TsM.js";import{r as J}from"./index-yIsmwZOr.js";import{g as K,a as Q,u as X,s as Y,c as Z,b as ee}from"./createSimplePaletteValueFilter-BVHFdhwo.js";import{T as i}from"./Typography-CMOwXBFg.js";import{B}from"./Box-BsxrtvkB.js";import{B as ne}from"./Button-CeoryglI.js";function oe(e){return K("MuiCardMedia",e)}Q("MuiCardMedia",["root","media","img"]);const te=e=>{const{classes:o,isMediaComponent:s,isImageComponent:r}=e;return ee({root:["root",s&&"media",r&&"img"]},oe,o)},re=Y("div",{name:"MuiCardMedia",slot:"Root",overridesResolver:(e,o)=>{const{ownerState:s}=e,{isMediaComponent:r,isImageComponent:t}=s;return[o.root,r&&o.media,t&&o.img]}})({display:"block",backgroundSize:"cover",backgroundRepeat:"no-repeat",backgroundPosition:"center",variants:[{props:{isMediaComponent:!0},style:{width:"100%"}},{props:{isImageComponent:!0},style:{objectFit:"cover"}}]}),se=["video","audio","picture","iframe","img"],ae=["picture","img"],ie=J.forwardRef(function(o,s){const r=X({props:o,name:"MuiCardMedia"}),{children:t,className:F,component:c="div",image:d,src:U,style:x,...W}=r,l=se.includes(c),q=!l&&d?{backgroundImage:`url("${d}")`,...x}:x,f={...r,component:c,isMediaComponent:l,isImageComponent:ae.includes(c)},V=te(f);return n.jsx(re,{className:Z(V.root,F),as:c,role:!l&&d?"img":void 0,ref:s,style:q,ownerState:f,src:l?d||U:void 0,...W,children:t})}),ce=(e,o=120)=>e.length<=o?e:e.substring(0,o).trim()+"...",O=({connector:e,onConnect:o,isConnecting:s})=>{const r=e.highlight||ce(e.description);return n.jsxs(N,{sx:{width:300,height:"100%",display:"flex",flexDirection:"column"},children:[n.jsx(ie,{component:"img",height:"140",image:e.logo,alt:`${e.display_name} logo`}),n.jsxs(z,{sx:{flexGrow:1},children:[n.jsx(i,{gutterBottom:!0,variant:"h5",component:"div",children:e.display_name}),n.jsx(B,{sx:{"& p":{margin:0,fontSize:"0.875rem",color:"text.secondary"},"& strong":{color:"text.primary"},"& em":{color:"text.secondary"},"& code":{backgroundColor:"action.hover",padding:"2px 4px",borderRadius:"4px",fontSize:"0.8rem"}},children:n.jsx(H,{remarkPlugins:[L],components:{p:({children:t})=>n.jsx(i,{variant:"body2",color:"text.secondary",children:t}),strong:({children:t})=>n.jsx(i,{component:"span",sx:{fontWeight:"bold",color:"text.primary"},children:t}),em:({children:t})=>n.jsx(i,{component:"span",sx:{fontStyle:"italic",color:"text.secondary"},children:t}),code:({children:t})=>n.jsx(i,{component:"code",sx:{backgroundColor:"action.hover",padding:"2px 4px",borderRadius:"4px",fontSize:"0.8rem",fontFamily:"monospace"},children:t})},children:r})})]}),n.jsx(A,{children:n.jsx(ne,{size:"small",color:"primary",onClick:()=>o(e.id),disabled:s,children:"Connect"})})]})},P=()=>n.jsxs(N,{sx:{maxWidth:345,height:"100%",display:"flex",flexDirection:"column"},children:[n.jsx(a,{variant:"rectangular",height:140}),n.jsxs(z,{sx:{flexGrow:1},children:[n.jsx(a,{variant:"text",height:32,width:"80%"}),n.jsxs(B,{sx:{mt:1},children:[n.jsx(a,{variant:"text",height:20}),n.jsx(a,{variant:"text",height:20}),n.jsx(a,{variant:"text",height:20,width:"60%"})]})]}),n.jsx(A,{children:n.jsx(a,{variant:"rectangular",width:80,height:30})})]});P.__docgenInfo={description:"Skeleton loader for the ConnectorCard",methods:[],displayName:"ConnectorCardSkeleton"};O.__docgenInfo={description:"Component to display a single connector with its details",methods:[],displayName:"ConnectorCard",props:{connector:{required:!0,tsType:{name:"Connector"},description:""},onConnect:{required:!0,tsType:{name:"signature",type:"function",raw:"(connectorId: string) => void",signature:{arguments:[{type:{name:"string"},name:"connectorId"}],return:{name:"void"}}},description:""},isConnecting:{required:!0,tsType:{name:"boolean"},description:""}}};const he={title:"Components/ConnectorCard",component:O,parameters:{layout:"centered"},tags:["autodocs"]},h={id:"google-calendar",version:1,state:y.ACTIVE,type:"oauth",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg",versions:1,states:[y.ACTIVE]},m={args:{connector:h,onConnect:e=>console.log(`Connect clicked for ${e}`),isConnecting:!1}},g={args:{connector:{...h,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`},onConnect:e=>console.log(`Connect clicked for ${e}`),isConnecting:!1}},p={args:{connector:h,onConnect:e=>console.log(`Connect clicked for ${e}`),isConnecting:!0}},u={args:{connector:{...h,description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events."},onConnect:e=>console.log(`Connect clicked for ${e}`),isConnecting:!1}},C={render:()=>n.jsx(P,{})};var v,k,j;m.parameters={...m.parameters,docs:{...(v=m.parameters)==null?void 0:v.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: false
  }
}`,...(j=(k=m.parameters)==null?void 0:k.docs)==null?void 0:j.source}}};var S,M,b;g.parameters={...g.parameters,docs:{...(S=g.parameters)==null?void 0:S.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\\n\\n• Event creation and management\\n• Meeting scheduling\\n• Reminder notifications\\n• Calendar sharing'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: false
  }
}`,...(b=(M=g.parameters)==null?void 0:M.docs)==null?void 0:b.source}}};var w,_,I;p.parameters={...p.parameters,docs:{...(w=p.parameters)==null?void 0:w.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: true
  }
}`,...(I=(_=p.parameters)==null?void 0:_.docs)==null?void 0:I.source}}};var T,E,G;u.parameters={...u.parameters,docs:{...(T=u.parameters)==null?void 0:T.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: false
  }
}`,...(G=(E=u.parameters)==null?void 0:E.docs)==null?void 0:G.source}}};var $,R,D;C.parameters={...C.parameters,docs:{...($=C.parameters)==null?void 0:$.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...(D=(R=C.parameters)==null?void 0:R.docs)==null?void 0:D.source}}};const xe=["Default","WithHighlight","Connecting","LongDescription","Skeleton"];export{p as Connecting,m as Default,u as LongDescription,C as Skeleton,g as WithHighlight,xe as __namedExportsOrder,he as default};
