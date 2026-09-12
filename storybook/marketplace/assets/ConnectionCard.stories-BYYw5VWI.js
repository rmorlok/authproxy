import{j as a}from"./jsx-runtime-BjG_zV1W.js";import{C as gn,a as un}from"./ConnectionCard-wxVwhmYF.js";import{C as rn,a as r}from"./connections-CPVoMvMW.js";import{P as cn,c as an,a as sn,b as dn}from"./connectionPresentation-BXj0Ghjo.js";import{c as w,a as o}from"./resources-D1r1Wyr4.js";import"./index-yIsmwZOr.js";import"./createSvgIcon-BzPbi2jQ.js";import"./createSimplePaletteValueFilter-CDpnWFvD.js";import"./theme-C544Z3EA.js";import"./index-BXrwOJ9g.js";import"./Box-DuDZjhmh.js";import"./Chip-0IuoHKYd.js";import"./Button-BqFH1kjI.js";import"./Typography-D32yZ0gM.js";import"./CardActions-CK4gHLtU.js";import"./MenuItem-BRDK3hIB.js";import"./index-M3uX8AIl.js";import"./IconButton-BY81ztgX.js";const Cn=(t,c)=>{const mn=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${t} logo"><rect width="280" height="140" rx="8" fill="${c}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(mn)}`},pn=t=>{const c=`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${t} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(c)}`},ln=Cn("Google Calendar","#1a73e8"),Sn=pn("Wide Format Systems"),n=w({displayName:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:{publicUrl:ln}}),x=w({displayName:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:{publicUrl:ln},hasConfigure:!0}),k=w({id:"wide-format-systems",displayName:"Wide Format Systems",highlight:"A wide logo should scale down inside the header without being cut off.",logo:{publicUrl:Sn}}),hn=an({reducer:{connectors:dn,connections:sn},preloadedState:{connectors:{items:[n],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null}}}),On={title:"Components/ConnectionCard",component:gn,parameters:{layout:"centered"},tags:["autodocs"],decorators:[t=>a.jsx(cn,{store:hn,children:a.jsx(t,{})})]},e=o({id:"123e4567-e89b-12d3-a456-426614174000",connector:n,state:r.CONFIGURED,healthState:rn.HEALTHY}),s={args:{connection:e,connector:n}},i={args:{connection:e,connector:n,highlightNew:!0}},d={args:{connection:o({id:e.metadata.id,connector:x}),connector:x}},l={args:{connection:o({id:e.metadata.id,connector:k}),connector:k}},m={args:{connection:o({id:e.metadata.id,connector:x,healthState:rn.UNHEALTHY}),connector:x}},g={args:{connection:o({id:e.metadata.id,connector:n,state:r.SETUP}),connector:n}},u={args:{connection:o({id:e.metadata.id,connector:n,state:r.DISABLED}),connector:n}},C={args:{connection:o({id:e.metadata.id,connector:n,state:r.DISCONNECTING}),connector:n}},p={args:{connection:o({id:e.metadata.id,connector:n,state:r.DISCONNECTED}),connector:n}},S={args:{connection:o({connector:w({id:"unknown-connector"})})}},h={args:{connection:o({id:e.metadata.id,connector:n,state:r.DISCONNECTING}),connector:n},decorators:[t=>{const c=an({reducer:{connectors:dn,connections:sn},preloadedState:{connectors:{items:[n],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!0,disconnectionError:null,currentTaskId:"task-123"}}});return a.jsx(cn,{store:c,children:a.jsx(t,{})})}]},f={render:()=>a.jsx(un,{})};var N,E,I;s.parameters={...s.parameters,docs:{...(N=s.parameters)==null?void 0:N.docs,source:{originalSource:`{
  args: {
    connection: mockConnection,
    connector: googleConnector
  }
}`,...(I=(E=s.parameters)==null?void 0:E.docs)==null?void 0:I.source}}};var y,D,F;i.parameters={...i.parameters,docs:{...(y=i.parameters)==null?void 0:y.docs,source:{originalSource:`{
  args: {
    connection: mockConnection,
    connector: googleConnector,
    highlightNew: true
  }
}`,...(F=(D=i.parameters)==null?void 0:D.docs)==null?void 0:F.source}}};var b,v,T;d.parameters={...d.parameters,docs:{...(b=d.parameters)==null?void 0:b.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: configurableConnector
    }),
    connector: configurableConnector
  }
}`,...(T=(v=d.parameters)==null?void 0:v.docs)==null?void 0:T.source}}};var U,G,L;l.parameters={...l.parameters,docs:{...(U=l.parameters)==null?void 0:U.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: wideConnector
    }),
    connector: wideConnector
  }
}`,...(L=(G=l.parameters)==null?void 0:G.docs)==null?void 0:L.source}}};var A,H,O;m.parameters={...m.parameters,docs:{...(A=m.parameters)==null?void 0:A.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: configurableConnector,
      healthState: ConnectionHealthState.UNHEALTHY
    }),
    connector: configurableConnector
  }
}`,...(O=(H=m.parameters)==null?void 0:H.docs)==null?void 0:O.source}}};var P,R,j;g.parameters={...g.parameters,docs:{...(P=g.parameters)==null?void 0:P.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: googleConnector,
      state: ConnectionState.SETUP
    }),
    connector: googleConnector
  }
}`,...(j=(R=g.parameters)==null?void 0:R.docs)==null?void 0:j.source}}};var W,$,B;u.parameters={...u.parameters,docs:{...(W=u.parameters)==null?void 0:W.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: googleConnector,
      state: ConnectionState.DISABLED
    }),
    connector: googleConnector
  }
}`,...(B=($=u.parameters)==null?void 0:$.docs)==null?void 0:B.source}}};var Y,_,z;C.parameters={...C.parameters,docs:{...(Y=C.parameters)==null?void 0:Y.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: googleConnector,
      state: ConnectionState.DISCONNECTING
    }),
    connector: googleConnector
  }
}`,...(z=(_=C.parameters)==null?void 0:_.docs)==null?void 0:z.source}}};var q,J,K;p.parameters={...p.parameters,docs:{...(q=p.parameters)==null?void 0:q.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: googleConnector,
      state: ConnectionState.DISCONNECTED
    }),
    connector: googleConnector
  }
}`,...(K=(J=p.parameters)==null?void 0:J.docs)==null?void 0:K.source}}};var M,Q,V;S.parameters={...S.parameters,docs:{...(M=S.parameters)==null?void 0:M.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      connector: connectorFixture({
        id: 'unknown-connector'
      })
    })
  }
}`,...(V=(Q=S.parameters)==null?void 0:Q.docs)==null?void 0:V.source}}};var X,Z,nn;h.parameters={...h.parameters,docs:{...(X=h.parameters)==null?void 0:X.docs,source:{originalSource:`{
  args: {
    connection: connectionFixture({
      id: mockConnection.metadata.id,
      connector: googleConnector,
      state: ConnectionState.DISCONNECTING
    }),
    connector: googleConnector
  },
  decorators: [Story => {
    const store = configureStore({
      reducer: {
        connectors: connectorsReducer,
        connections: connectionsReducer
      },
      preloadedState: {
        connectors: {
          items: [googleConnector],
          status: 'succeeded',
          error: null
        },
        connections: {
          items: [],
          status: 'idle',
          error: null,
          initiatingConnection: false,
          initiationError: null,
          disconnectingConnection: true,
          disconnectionError: null,
          currentTaskId: 'task-123'
        }
      }
    });
    return <Provider store={store}>
          <Story />
        </Provider>;
  }]
}`,...(nn=(Z=h.parameters)==null?void 0:Z.docs)==null?void 0:nn.source}}};var on,en,tn;f.parameters={...f.parameters,docs:{...(on=f.parameters)==null?void 0:on.docs,source:{originalSource:`{
  render: () => <ConnectionCardSkeleton />
}`,...(tn=(en=f.parameters)==null?void 0:en.docs)==null?void 0:tn.source}}};const Pn=["Connected","NewlyConnected","ConnectedConfigurable","WideLogo","Unhealthy","Created","Failed","Disconnecting","Disconnected","UnknownConnector","WithTaskInProgress","Skeleton"];export{s as Connected,d as ConnectedConfigurable,g as Created,p as Disconnected,C as Disconnecting,u as Failed,i as NewlyConnected,f as Skeleton,m as Unhealthy,S as UnknownConnector,l as WideLogo,h as WithTaskInProgress,Pn as __namedExportsOrder,On as default};
