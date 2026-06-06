import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{C as tn,P as K,c as Q,a as rn,b as X,d as nn}from"./ConnectionCard-D1fvVuJx.js";import"./client-Bd8K-UT9.js";import{C as h}from"./index-C7dhk8kh.js";import{C as en,a as e}from"./connections-DBlS__Tv.js";import"./index-yIsmwZOr.js";import"./createSvgIcon-YAXHmVIe.js";import"./createSimplePaletteValueFilter-DRANIdZ4.js";import"./theme-D4YlWSlu.js";import"./IconButton-CLik2w7b.js";import"./Button-Ct-3TB0V.js";import"./Typography-zR6YQtWt.js";import"./Box-qKFn2-pG.js";import"./Chip-oJjan0S7.js";import"./index-M3uX8AIl.js";const cn=(o,C)=>{const on=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${o} logo"><rect width="280" height="140" rx="8" fill="${C}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(on)}`},S=cn("Google Calendar","#1a73e8"),an=Q({reducer:{connectors:nn,connections:X},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:S}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null}}}),Dn={title:"Components/ConnectionCard",component:tn,parameters:{layout:"centered"},tags:["autodocs"],decorators:[o=>t.jsx(K,{store:an,children:t.jsx(o,{})})]},n={id:"123e4567-e89b-12d3-a456-426614174000",connector:{type:"google-calendar",versions:0,states:[h.PRIMARY],id:"923e4567-e89b-12d3-a456-426614174009",version:0,state:h.PRIMARY,display_name:"Google Calendar",description:"A google calendar connector",logo:S},state:e.CONFIGURED,health_state:en.HEALTHY,created_at:"2023-04-01T12:00:00Z",updated_at:"2023-04-01T12:00:00Z"},r={args:{connection:n}},c={args:{connection:n,highlightNew:!0}},a={args:{connection:{...n,connector:{...n.connector,has_configure:!0}}}},s={args:{connection:{...n,connector:{...n.connector,has_configure:!0},health_state:en.UNHEALTHY}}},i={args:{connection:{...n,state:e.SETUP}}},d={args:{connection:{...n,state:e.DISABLED}}},l={args:{connection:{...n,state:e.DISCONNECTING}}},m={args:{connection:{...n,state:e.DISCONNECTED}}},g={args:{connection:{...n,connector_id:"unknown-connector"}}},u={args:{connection:{...n,state:e.DISCONNECTING}},decorators:[o=>{const C=Q({reducer:{connectors:nn,connections:X},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:S}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!0,disconnectionError:null,currentTaskId:"task-123"}}});return t.jsx(K,{store:C,children:t.jsx(o,{})})}]},p={render:()=>t.jsx(rn,{})};var k,f,E;r.parameters={...r.parameters,docs:{...(k=r.parameters)==null?void 0:k.docs,source:{originalSource:`{
  args: {
    connection: mockConnection
  }
}`,...(E=(f=r.parameters)==null?void 0:f.docs)==null?void 0:E.source}}};var N,I,D;c.parameters={...c.parameters,docs:{...(N=c.parameters)==null?void 0:N.docs,source:{originalSource:`{
  args: {
    connection: mockConnection,
    highlightNew: true
  }
}`,...(D=(I=c.parameters)==null?void 0:I.docs)==null?void 0:D.source}}};var T,_,x;a.parameters={...a.parameters,docs:{...(T=a.parameters)==null?void 0:T.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true
      }
    }
  }
}`,...(x=(_=a.parameters)==null?void 0:_.docs)==null?void 0:x.source}}};var y,w,v;s.parameters={...s.parameters,docs:{...(y=s.parameters)==null?void 0:y.docs,source:{originalSource:`{
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
}`,...(v=(w=s.parameters)==null?void 0:w.docs)==null?void 0:v.source}}};var G,R,U;i.parameters={...i.parameters,docs:{...(G=i.parameters)==null?void 0:G.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.SETUP
    }
  }
}`,...(U=(R=i.parameters)==null?void 0:R.docs)==null?void 0:U.source}}};var P,A,H;d.parameters={...d.parameters,docs:{...(P=d.parameters)==null?void 0:P.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISABLED
    }
  }
}`,...(H=(A=d.parameters)==null?void 0:A.docs)==null?void 0:H.source}}};var O,b,j;l.parameters={...l.parameters,docs:{...(O=l.parameters)==null?void 0:O.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  }
}`,...(j=(b=l.parameters)==null?void 0:b.docs)==null?void 0:j.source}}};var L,Y,B;m.parameters={...m.parameters,docs:{...(L=m.parameters)==null?void 0:L.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTED
    }
  }
}`,...(B=(Y=m.parameters)==null?void 0:Y.docs)==null?void 0:B.source}}};var F,$,M;g.parameters={...g.parameters,docs:{...(F=g.parameters)==null?void 0:F.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector_id: 'unknown-connector'
    }
  }
}`,...(M=($=g.parameters)==null?void 0:$.docs)==null?void 0:M.source}}};var W,Z,z;u.parameters={...u.parameters,docs:{...(W=u.parameters)==null?void 0:W.docs,source:{originalSource:`{
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
}`,...(z=(Z=u.parameters)==null?void 0:Z.docs)==null?void 0:z.source}}};var V,q,J;p.parameters={...p.parameters,docs:{...(V=p.parameters)==null?void 0:V.docs,source:{originalSource:`{
  render: () => <ConnectionCardSkeleton />
}`,...(J=(q=p.parameters)==null?void 0:q.docs)==null?void 0:J.source}}};const Tn=["Connected","NewlyConnected","ConnectedConfigurable","Unhealthy","Created","Failed","Disconnecting","Disconnected","UnknownConnector","WithTaskInProgress","Skeleton"];export{r as Connected,a as ConnectedConfigurable,i as Created,m as Disconnected,l as Disconnecting,d as Failed,c as NewlyConnected,p as Skeleton,s as Unhealthy,g as UnknownConnector,u as WithTaskInProgress,Tn as __namedExportsOrder,Dn as default};
