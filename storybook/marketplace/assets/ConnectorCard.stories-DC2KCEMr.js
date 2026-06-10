import{j as F}from"./jsx-runtime-BjG_zV1W.js";import{C as A,a as E}from"./ConnectorCard-BjbM7WNq.js";import"./client-CUR1Dy-U.js";import{C as l}from"./ConnectorLogo-De4NpjV6.js";import"./theme-D4YlWSlu.js";import"./createSimplePaletteValueFilter-DRANIdZ4.js";import"./index-yIsmwZOr.js";import"./Typography-zR6YQtWt.js";import"./Box-qKFn2-pG.js";import"./Button-ClhtrhF9.js";const I=(e,c)=>{const W=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${c}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(W)}`},L=e=>{const c=`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${e} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(c)}`},O={title:"Components/ConnectorCard",component:A,parameters:{layout:"centered"},tags:["autodocs"]},o={id:"google-calendar",version:1,state:l.ACTIVE,type:"oauth",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",highlight:"Manage events and appointments from Google Calendar.",logo:I("Google Calendar","#1a73e8"),versions:1,states:[l.ACTIVE]},n={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},t={args:{connector:{...o,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},i={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!0}},s={args:{connector:{...o,description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.",highlight:"Short marketplace highlight stays on the card while the long description belongs on the overview page."},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},a={args:{connector:{...o,display_name:"Wide Format Systems",highlight:"A wide logo should scale down inside the card without being cut off.",logo:L("Wide Format Systems")},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},r={render:()=>F.jsx(E,{})};var d,g,m;n.parameters={...n.parameters,docs:{...(d=n.parameters)==null?void 0:d.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(m=(g=n.parameters)==null?void 0:g.docs)==null?void 0:m.source}}};var h,p,C;t.parameters={...t.parameters,docs:{...(h=t.parameters)==null?void 0:h.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\\n\\n• Event creation and management\\n• Meeting scheduling\\n• Reminder notifications\\n• Calendar sharing'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(C=(p=t.parameters)==null?void 0:p.docs)==null?void 0:C.source}}};var f,u,k;i.parameters={...i.parameters,docs:{...(f=i.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: true
  }
}`,...(k=(u=i.parameters)==null?void 0:u.docs)==null?void 0:k.source}}};var w,v,D;s.parameters={...s.parameters,docs:{...(w=s.parameters)==null?void 0:w.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.',
      highlight: 'Short marketplace highlight stays on the card while the long description belongs on the overview page.'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(D=(v=s.parameters)==null?void 0:v.docs)==null?void 0:D.source}}};var y,$,x;a.parameters={...a.parameters,docs:{...(y=a.parameters)==null?void 0:y.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      display_name: 'Wide Format Systems',
      highlight: 'A wide logo should scale down inside the card without being cut off.',
      logo: wideLogoDataUri('Wide Format Systems')
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(x=($=a.parameters)==null?void 0:$.docs)==null?void 0:x.source}}};var S,b,G;r.parameters={...r.parameters,docs:{...(S=r.parameters)==null?void 0:S.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...(G=(b=r.parameters)==null?void 0:b.docs)==null?void 0:G.source}}};const q=["Default","WithHighlight","Connecting","LongDescription","WideLogo","Skeleton"];export{i as Connecting,n as Default,s as LongDescription,r as Skeleton,a as WideLogo,t as WithHighlight,q as __namedExportsOrder,O as default};
