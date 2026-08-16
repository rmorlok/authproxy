import{C as v}from"./ConnectionFormStep-Em9wryDt.js";import{f as s}from"./index-Droui3yq.js";import"./jsx-runtime-BjG_zV1W.js";import"./index-yIsmwZOr.js";import"./createSimplePaletteValueFilter-CvlNngyw.js";import"./Chip-BT1MOgdz.js";import"./createSvgIcon-ByfhJewD.js";import"./Button-qoqN5xvQ.js";import"./Typography-USnMzuFJ.js";import"./Box-CWJIOfo9.js";import"./connections-DMQz4Kw3.js";import"./index-M3uX8AIl.js";import"./client-6EFRD0gB.js";import"./useThemeProps-2qsqrhXJ.js";import"./IconButton-T-YZe696.js";import"./Close-BpyWLyqp.js";import"./Stack-L6r6t084.js";import"./theme-CKuQCuBO.js";const G={title:"Components/ConnectionFormStep",component:v,parameters:{layout:"centered"},tags:["autodocs"],args:{onSubmit:s(),onCancel:s(),isSubmitting:!1,connectionId:"cxn_test550e8400abcde"}},e={args:{jsonSchema:{type:"object",properties:{apiKey:{type:"string",title:"API Key",description:"Your API key for the service"},region:{type:"string",title:"Region",enum:["us-east-1","us-west-2","eu-west-1","ap-southeast-1"]}},required:["apiKey","region"]},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/apiKey"},{type:"Control",scope:"#/properties/region"}]}}},t={args:{jsonSchema:{type:"object",properties:{name:{type:"string",title:"Connection Name",minLength:3,maxLength:50},environment:{type:"string",title:"Environment",enum:["production","staging","development"],default:"development"},credentials:{type:"object",title:"Credentials",properties:{username:{type:"string",title:"Username"},password:{type:"string",title:"Password"}},required:["username","password"]},enableLogging:{type:"boolean",title:"Enable Request Logging",default:!0},maxRetries:{type:"integer",title:"Max Retries",minimum:0,maximum:10,default:3}},required:["name","environment","credentials"]},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/name"},{type:"Control",scope:"#/properties/environment"},{type:"Group",label:"Authentication",elements:[{type:"Control",scope:"#/properties/credentials/properties/username"},{type:"Control",scope:"#/properties/credentials/properties/password",options:{format:"password"}}]},{type:"HorizontalLayout",elements:[{type:"Control",scope:"#/properties/enableLogging"},{type:"Control",scope:"#/properties/maxRetries"}]}]}}},r={args:{...e.args,isSubmitting:!0}},n={args:{connectionId:"cxn_google_calendar",stepTitle:"Select a Calendar",stepDescription:"Your current selection is shown. Save it unchanged or choose another calendar.",jsonSchema:{type:"object",properties:{calendar:{type:"string",title:"Calendar",enum:["primary","product","support"]},syncDirection:{type:"string",title:"Sync direction",enum:["two-way","import-only","export-only"]}},required:["calendar","syncDirection"]},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar"},{type:"Control",scope:"#/properties/syncDirection"}]},initialData:{calendar:"product",syncDirection:"two-way"}}},o={args:{...n.args,stepDescription:"Before the fix, reconfigure opened without the stored selections.",initialData:void 0}},i={args:{jsonSchema:{type:"object",properties:{token:{type:"string",title:"Access Token",description:"Paste your personal access token here"}},required:["token"]},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/token"}]}}};var p,a,c;e.parameters={...e.parameters,docs:{...(p=e.parameters)==null?void 0:p.docs,source:{originalSource:`{
  args: {
    jsonSchema: {
      type: 'object',
      properties: {
        apiKey: {
          type: 'string',
          title: 'API Key',
          description: 'Your API key for the service'
        },
        region: {
          type: 'string',
          title: 'Region',
          enum: ['us-east-1', 'us-west-2', 'eu-west-1', 'ap-southeast-1']
        }
      },
      required: ['apiKey', 'region']
    },
    uiSchema: {
      type: 'VerticalLayout',
      elements: [{
        type: 'Control',
        scope: '#/properties/apiKey'
      }, {
        type: 'Control',
        scope: '#/properties/region'
      }]
    }
  }
}`,...(c=(a=e.parameters)==null?void 0:a.docs)==null?void 0:c.source}}};var m,l,u;t.parameters={...t.parameters,docs:{...(m=t.parameters)==null?void 0:m.docs,source:{originalSource:`{
  args: {
    jsonSchema: {
      type: 'object',
      properties: {
        name: {
          type: 'string',
          title: 'Connection Name',
          minLength: 3,
          maxLength: 50
        },
        environment: {
          type: 'string',
          title: 'Environment',
          enum: ['production', 'staging', 'development'],
          default: 'development'
        },
        credentials: {
          type: 'object',
          title: 'Credentials',
          properties: {
            username: {
              type: 'string',
              title: 'Username'
            },
            password: {
              type: 'string',
              title: 'Password'
            }
          },
          required: ['username', 'password']
        },
        enableLogging: {
          type: 'boolean',
          title: 'Enable Request Logging',
          default: true
        },
        maxRetries: {
          type: 'integer',
          title: 'Max Retries',
          minimum: 0,
          maximum: 10,
          default: 3
        }
      },
      required: ['name', 'environment', 'credentials']
    },
    uiSchema: {
      type: 'VerticalLayout',
      elements: [{
        type: 'Control',
        scope: '#/properties/name'
      }, {
        type: 'Control',
        scope: '#/properties/environment'
      }, {
        type: 'Group',
        label: 'Authentication',
        elements: [{
          type: 'Control',
          scope: '#/properties/credentials/properties/username'
        }, {
          type: 'Control',
          scope: '#/properties/credentials/properties/password',
          options: {
            format: 'password'
          }
        }]
      }, {
        type: 'HorizontalLayout',
        elements: [{
          type: 'Control',
          scope: '#/properties/enableLogging'
        }, {
          type: 'Control',
          scope: '#/properties/maxRetries'
        }]
      }]
    }
  }
}`,...(u=(l=t.parameters)==null?void 0:l.docs)==null?void 0:u.source}}};var y,d,g;r.parameters={...r.parameters,docs:{...(y=r.parameters)==null?void 0:y.docs,source:{originalSource:`{
  args: {
    ...SimpleTextForm.args,
    isSubmitting: true
  }
}`,...(g=(d=r.parameters)==null?void 0:d.docs)==null?void 0:g.source}}};var h,S,C;n.parameters={...n.parameters,docs:{...(h=n.parameters)==null?void 0:h.docs,source:{originalSource:`{
  args: {
    connectionId: 'cxn_google_calendar',
    stepTitle: 'Select a Calendar',
    stepDescription: 'Your current selection is shown. Save it unchanged or choose another calendar.',
    jsonSchema: {
      type: 'object',
      properties: {
        calendar: {
          type: 'string',
          title: 'Calendar',
          enum: ['primary', 'product', 'support']
        },
        syncDirection: {
          type: 'string',
          title: 'Sync direction',
          enum: ['two-way', 'import-only', 'export-only']
        }
      },
      required: ['calendar', 'syncDirection']
    },
    uiSchema: {
      type: 'VerticalLayout',
      elements: [{
        type: 'Control',
        scope: '#/properties/calendar'
      }, {
        type: 'Control',
        scope: '#/properties/syncDirection'
      }]
    },
    initialData: {
      calendar: 'product',
      syncDirection: 'two-way'
    }
  }
}`,...(C=(S=n.parameters)==null?void 0:S.docs)==null?void 0:C.source}}};var x,f,b;o.parameters={...o.parameters,docs:{...(x=o.parameters)==null?void 0:x.docs,source:{originalSource:`{
  args: {
    ...ReconfigureWithExistingSelections.args,
    stepDescription: 'Before the fix, reconfigure opened without the stored selections.',
    initialData: undefined
  }
}`,...(b=(f=o.parameters)==null?void 0:f.docs)==null?void 0:b.source}}};var w,L,j;i.parameters={...i.parameters,docs:{...(w=i.parameters)==null?void 0:w.docs,source:{originalSource:`{
  args: {
    jsonSchema: {
      type: 'object',
      properties: {
        token: {
          type: 'string',
          title: 'Access Token',
          description: 'Paste your personal access token here'
        }
      },
      required: ['token']
    },
    uiSchema: {
      type: 'VerticalLayout',
      elements: [{
        type: 'Control',
        scope: '#/properties/token'
      }]
    }
  }
}`,...(j=(L=i.parameters)==null?void 0:L.docs)==null?void 0:j.source}}};const H=["SimpleTextForm","ComplexForm","Submitting","ReconfigureWithExistingSelections","ReconfigureWithoutExistingSelections","MinimalForm"];export{t as ComplexForm,i as MinimalForm,n as ReconfigureWithExistingSelections,o as ReconfigureWithoutExistingSelections,e as SimpleTextForm,r as Submitting,H as __namedExportsOrder,G as default};
