import{j as e}from"./jsx-runtime-BjG_zV1W.js";import{C as Q,P as Z,c as V,a as X,b as q,d as z}from"./ConnectionCard-DGvu8dYk.js";import"./client-CitdaXMW.js";import{C as g}from"./index-C0v9_cyh.js";import{C as J,a as o}from"./connections-B1_FnVaK.js";import"./index-yIsmwZOr.js";import"./IconButton-CRpx-Pp7.js";import"./createSimplePaletteValueFilter-CffhNcpo.js";import"./Button-BU-5bqsh.js";import"./Typography-Bi0mUh4H.js";import"./Box-Dxb9LzqI.js";import"./index-M3uX8AIl.js";const $=V({reducer:{connectors:z,connections:q},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg"}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null}}}),pn={title:"Components/ConnectionCard",component:Q,parameters:{layout:"centered"},tags:["autodocs"],decorators:[p=>e.jsx(Z,{store:$,children:e.jsx(p,{})})]},n={id:"123e4567-e89b-12d3-a456-426614174000",connector:{type:"google-calendar",versions:0,states:[g.PRIMARY],id:"923e4567-e89b-12d3-a456-426614174009",version:0,state:g.PRIMARY,display_name:"Google Calendar",description:"A google calendar connector",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg"},state:o.CONFIGURED,health_state:J.HEALTHY,created_at:"2023-04-01T12:00:00Z",updated_at:"2023-04-01T12:00:00Z"},t={args:{connection:n}},r={args:{connection:{...n,connector:{...n.connector,has_configure:!0}}}},c={args:{connection:{...n,connector:{...n.connector,has_configure:!0},health_state:J.UNHEALTHY}}},a={args:{connection:{...n,state:o.SETUP}}},s={args:{connection:{...n,state:o.DISABLED}}},i={args:{connection:{...n,state:o.DISCONNECTING}}},d={args:{connection:{...n,state:o.DISCONNECTED}}},l={args:{connection:{...n,connector_id:"unknown-connector"}}},m={args:{connection:{...n,state:o.DISCONNECTING}},decorators:[p=>{const K=V({reducer:{connectors:z,connections:q},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg"}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!0,disconnectionError:null,currentTaskId:"task-123"}}});return e.jsx(Z,{store:K,children:e.jsx(p,{})})}]},u={render:()=>e.jsx(X,{})};var C,S,k;t.parameters={...t.parameters,docs:{...(C=t.parameters)==null?void 0:C.docs,source:{originalSource:`{
  args: {
    connection: mockConnection
  }
}`,...(k=(S=t.parameters)==null?void 0:S.docs)==null?void 0:k.source}}};var _,h,E;r.parameters={...r.parameters,docs:{...(_=r.parameters)==null?void 0:_.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true
      }
    }
  }
}`,...(E=(h=r.parameters)==null?void 0:h.docs)==null?void 0:E.source}}};var I,N,f;c.parameters={...c.parameters,docs:{...(I=c.parameters)==null?void 0:I.docs,source:{originalSource:`{
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
}`,...(f=(N=c.parameters)==null?void 0:N.docs)==null?void 0:f.source}}};var T,D,G;a.parameters={...a.parameters,docs:{...(T=a.parameters)==null?void 0:T.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.SETUP
    }
  }
}`,...(G=(D=a.parameters)==null?void 0:D.docs)==null?void 0:G.source}}};var y,v,w;s.parameters={...s.parameters,docs:{...(y=s.parameters)==null?void 0:y.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISABLED
    }
  }
}`,...(w=(v=s.parameters)==null?void 0:v.docs)==null?void 0:w.source}}};var P,R,x;i.parameters={...i.parameters,docs:{...(P=i.parameters)==null?void 0:P.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  }
}`,...(x=(R=i.parameters)==null?void 0:R.docs)==null?void 0:x.source}}};var U,A,H;d.parameters={...d.parameters,docs:{...(U=d.parameters)==null?void 0:U.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTED
    }
  }
}`,...(H=(A=d.parameters)==null?void 0:A.docs)==null?void 0:H.source}}};var O,j,b;l.parameters={...l.parameters,docs:{...(O=l.parameters)==null?void 0:O.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector_id: 'unknown-connector'
    }
  }
}`,...(b=(j=l.parameters)==null?void 0:j.docs)==null?void 0:b.source}}};var L,Y,F;m.parameters={...m.parameters,docs:{...(L=m.parameters)==null?void 0:L.docs,source:{originalSource:`{
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
            logo: 'https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg'
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
}`,...(F=(Y=m.parameters)==null?void 0:Y.docs)==null?void 0:F.source}}};var B,M,W;u.parameters={...u.parameters,docs:{...(B=u.parameters)==null?void 0:B.docs,source:{originalSource:`{
  render: () => <ConnectionCardSkeleton />
}`,...(W=(M=u.parameters)==null?void 0:M.docs)==null?void 0:W.source}}};const gn=["Connected","ConnectedConfigurable","Unhealthy","Created","Failed","Disconnecting","Disconnected","UnknownConnector","WithTaskInProgress","Skeleton"];export{t as Connected,r as ConnectedConfigurable,a as Created,d as Disconnected,i as Disconnecting,s as Failed,u as Skeleton,c as Unhealthy,l as UnknownConnector,m as WithTaskInProgress,gn as __namedExportsOrder,pn as default};
