import{j as G}from"./jsx-runtime-BjG_zV1W.js";import{C as w,a as E}from"./ConnectorCard-DPFwfeEv.js";import"./client-HmSzpqVp.js";import{C as s}from"./index-Cck04WQ2.js";import"./index-yIsmwZOr.js";import"./createSimplePaletteValueFilter-jJDq419X.js";import"./Typography-Cb-G69Z7.js";import"./Box-Rx5U04Ep.js";import"./Button-C_mwJF14.js";const F={title:"Components/ConnectorCard",component:w,parameters:{layout:"centered"},tags:["autodocs"]},c={id:"google-calendar",version:1,state:s.ACTIVE,type:"oauth",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg",versions:1,states:[s.ACTIVE]},e={args:{connector:c,onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!1}},o={args:{connector:{...c,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`},onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!1}},t={args:{connector:c,onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!0}},r={args:{connector:{...c,description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events."},onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!1}},a={render:()=>G.jsx(E,{})};var i,l,d;e.parameters={...e.parameters,docs:{...(i=e.parameters)==null?void 0:i.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: false
  }
}`,...(d=(l=e.parameters)==null?void 0:l.docs)==null?void 0:d.source}}};var g,m,p;o.parameters={...o.parameters,docs:{...(g=o.parameters)==null?void 0:g.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\\n\\n• Event creation and management\\n• Meeting scheduling\\n• Reminder notifications\\n• Calendar sharing'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: false
  }
}`,...(p=(m=o.parameters)==null?void 0:m.docs)==null?void 0:p.source}}};var C,u,h;t.parameters={...t.parameters,docs:{...(C=t.parameters)==null?void 0:C.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: true
  }
}`,...(h=(u=t.parameters)==null?void 0:u.docs)==null?void 0:h.source}}};var f,k,v;r.parameters={...r.parameters,docs:{...(f=r.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: false
  }
}`,...(v=(k=r.parameters)==null?void 0:k.docs)==null?void 0:v.source}}};var y,S,$;a.parameters={...a.parameters,docs:{...(y=a.parameters)==null?void 0:y.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...($=(S=a.parameters)==null?void 0:S.docs)==null?void 0:$.source}}};const H=["Default","WithHighlight","Connecting","LongDescription","Skeleton"];export{t as Connecting,e as Default,r as LongDescription,a as Skeleton,o as WithHighlight,H as __namedExportsOrder,F as default};
