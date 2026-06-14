import{j as r}from"./jsx-runtime-BjG_zV1W.js";import{C as sn,P as en,c as on,a as dn,b as tn,d as rn}from"./ConnectionCard-CtfITcOy.js";import"./client-CkodnXSl.js";import{C as f}from"./ConnectorLogo-kojr_uM7.js";import{C as cn,a as o}from"./connections-B9-zS0cD.js";import"./index-yIsmwZOr.js";import"./createSvgIcon-YAXHmVIe.js";import"./createSimplePaletteValueFilter-DRANIdZ4.js";import"./theme-BLqDajYR.js";import"./IconButton-B1e-KfmS.js";import"./Button-CH0iXqSo.js";import"./Typography-C9WFTw_d.js";import"./Box-qKFn2-pG.js";import"./Chip-Ck5A9lF8.js";import"./index-M3uX8AIl.js";const ln=(e,t)=>{const an=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${t}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(an)}`},mn=e=>{const t=`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${e} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(t)}`},S=ln("Google Calendar","#1a73e8"),gn=mn("Wide Format Systems"),un=on({reducer:{connectors:rn,connections:tn},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:S}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null}}}),Tn={title:"Components/ConnectionCard",component:sn,parameters:{layout:"centered"},tags:["autodocs"],decorators:[e=>r.jsx(en,{store:un,children:r.jsx(e,{})})]},n={id:"123e4567-e89b-12d3-a456-426614174000",connector:{type:"google-calendar",versions:0,states:[f.PRIMARY],id:"923e4567-e89b-12d3-a456-426614174009",version:0,state:f.PRIMARY,display_name:"Google Calendar",description:"A google calendar connector",logo:S},state:o.CONFIGURED,health_state:cn.HEALTHY,created_at:"2023-04-01T12:00:00Z",updated_at:"2023-04-01T12:00:00Z"},c={args:{connection:n}},a={args:{connection:n,highlightNew:!0}},s={args:{connection:{...n,connector:{...n.connector,has_configure:!0}}}},i={args:{connection:{...n,connector:{...n.connector,display_name:"Wide Format Systems",highlight:"A wide logo should scale down inside the header without being cut off.",logo:gn}}}},d={args:{connection:{...n,connector:{...n.connector,has_configure:!0},health_state:cn.UNHEALTHY}}},l={args:{connection:{...n,state:o.SETUP}}},m={args:{connection:{...n,state:o.DISABLED}}},g={args:{connection:{...n,state:o.DISCONNECTING}}},u={args:{connection:{...n,state:o.DISCONNECTED}}},p={args:{connection:{...n,connector_id:"unknown-connector"}}},C={args:{connection:{...n,state:o.DISCONNECTING}},decorators:[e=>{const t=on({reducer:{connectors:rn,connections:tn},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:S}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!0,disconnectionError:null,currentTaskId:"task-123"}}});return r.jsx(en,{store:t,children:r.jsx(e,{})})}]},h={render:()=>r.jsx(dn,{})};var w,k,y;c.parameters={...c.parameters,docs:{...(w=c.parameters)==null?void 0:w.docs,source:{originalSource:`{
  args: {
    connection: mockConnection
  }
}`,...(y=(k=c.parameters)==null?void 0:k.docs)==null?void 0:y.source}}};var x,E,I;a.parameters={...a.parameters,docs:{...(x=a.parameters)==null?void 0:x.docs,source:{originalSource:`{
  args: {
    connection: mockConnection,
    highlightNew: true
  }
}`,...(I=(E=a.parameters)==null?void 0:E.docs)==null?void 0:I.source}}};var N,v,_;s.parameters={...s.parameters,docs:{...(N=s.parameters)==null?void 0:N.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true
      }
    }
  }
}`,...(_=(v=s.parameters)==null?void 0:v.docs)==null?void 0:_.source}}};var D,T,G;i.parameters={...i.parameters,docs:{...(D=i.parameters)==null?void 0:D.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        display_name: 'Wide Format Systems',
        highlight: 'A wide logo should scale down inside the header without being cut off.',
        logo: wideFormatLogo
      }
    }
  }
}`,...(G=(T=i.parameters)==null?void 0:T.docs)==null?void 0:G.source}}};var U,A,L;d.parameters={...d.parameters,docs:{...(U=d.parameters)==null?void 0:U.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true
      },
      health_state: ConnectionHealthState.UNHEALTHY
    }
  }
}`,...(L=(A=d.parameters)==null?void 0:A.docs)==null?void 0:L.source}}};var R,b,P;l.parameters={...l.parameters,docs:{...(R=l.parameters)==null?void 0:R.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.SETUP
    }
  }
}`,...(P=(b=l.parameters)==null?void 0:b.docs)==null?void 0:P.source}}};var F,H,O;m.parameters={...m.parameters,docs:{...(F=m.parameters)==null?void 0:F.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISABLED
    }
  }
}`,...(O=(H=m.parameters)==null?void 0:H.docs)==null?void 0:O.source}}};var W,j,Y;g.parameters={...g.parameters,docs:{...(W=g.parameters)==null?void 0:W.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  }
}`,...(Y=(j=g.parameters)==null?void 0:j.docs)==null?void 0:Y.source}}};var $,B,z;u.parameters={...u.parameters,docs:{...($=u.parameters)==null?void 0:$.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTED
    }
  }
}`,...(z=(B=u.parameters)==null?void 0:B.docs)==null?void 0:z.source}}};var M,Z,V;p.parameters={...p.parameters,docs:{...(M=p.parameters)==null?void 0:M.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector_id: 'unknown-connector'
    }
  }
}`,...(V=(Z=p.parameters)==null?void 0:Z.docs)==null?void 0:V.source}}};var q,J,K;C.parameters={...C.parameters,docs:{...(q=C.parameters)==null?void 0:q.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  },
  decorators: [Story => {
    const store = configureStore({
      reducer: {
        connectors: connectorsReducer,
        connections: connectionsReducer
      },
      preloadedState: {
        connectors: {
          items: [{
            id: 'google-calendar',
            display_name: 'Google Calendar',
            description: 'Connect to your Google Calendar to manage events and appointments.',
            logo: googleCalendarLogo
          }],
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
}`,...(K=(J=C.parameters)==null?void 0:J.docs)==null?void 0:K.source}}};var Q,X,nn;h.parameters={...h.parameters,docs:{...(Q=h.parameters)==null?void 0:Q.docs,source:{originalSource:`{
  render: () => <ConnectionCardSkeleton />
}`,...(nn=(X=h.parameters)==null?void 0:X.docs)==null?void 0:nn.source}}};const Gn=["Connected","NewlyConnected","ConnectedConfigurable","WideLogo","Unhealthy","Created","Failed","Disconnecting","Disconnected","UnknownConnector","WithTaskInProgress","Skeleton"];export{c as Connected,s as ConnectedConfigurable,l as Created,u as Disconnected,g as Disconnecting,m as Failed,a as NewlyConnected,h as Skeleton,d as Unhealthy,p as UnknownConnector,i as WideLogo,C as WithTaskInProgress,Gn as __namedExportsOrder,Tn as default};
