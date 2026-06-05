import{j as G}from"./jsx-runtime-BjG_zV1W.js";import{C as E,a as D}from"./ConnectorCard-DO7VbSpU.js";import"./client-0PMYh2xf.js";import{C as c}from"./index-BcFHX6i3.js";import"./index-yIsmwZOr.js";import"./theme-D4YlWSlu.js";import"./createSimplePaletteValueFilter-DRANIdZ4.js";import"./Typography-zR6YQtWt.js";import"./Box-qKFn2-pG.js";import"./Button-DiNVVHLo.js";const b=(n,S)=>{const $=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${n} logo"><rect width="280" height="140" rx="8" fill="${S}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent($)}`},M={title:"Components/ConnectorCard",component:E,parameters:{layout:"centered"},tags:["autodocs"]},s={id:"google-calendar",version:1,state:c.ACTIVE,type:"oauth",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:b("Google Calendar","#1a73e8"),versions:1,states:[c.ACTIVE]},e={args:{connector:s,onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!1}},o={args:{connector:{...s,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`},onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!1}},t={args:{connector:s,onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!0}},r={args:{connector:{...s,description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events."},onConnect:n=>console.log(`Connect clicked for ${n}`),isConnecting:!1}},a={render:()=>G.jsx(D,{})};var i,l,d;e.parameters={...e.parameters,docs:{...(i=e.parameters)==null?void 0:i.docs,source:{originalSource:`{
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
}`,...(h=(u=t.parameters)==null?void 0:u.docs)==null?void 0:h.source}}};var f,v,k;r.parameters={...r.parameters,docs:{...(f=r.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    isConnecting: false
  }
}`,...(k=(v=r.parameters)==null?void 0:v.docs)==null?void 0:k.source}}};var y,x,w;a.parameters={...a.parameters,docs:{...(y=a.parameters)==null?void 0:y.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...(w=(x=a.parameters)==null?void 0:x.docs)==null?void 0:w.source}}};const U=["Default","WithHighlight","Connecting","LongDescription","Skeleton"];export{t as Connecting,e as Default,r as LongDescription,a as Skeleton,o as WithHighlight,U as __namedExportsOrder,M as default};
