import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{r as p}from"./index-yIsmwZOr.js";import{u as Be,d as v,H as xo,I as yo,J as vo,K as So,L as jo,M as wo,N as ko,O as en,Q as Me,T as Ao,U as Io,C as Ft,E as Vt,F as $t,G as Ht,s as Eo,e as To,f as Po,z as Ro,g as Lo,h as No,i as Mo,V as Do,W as Oo,X as zo,Y as Fo,j as B,l as nn,Z as Vo,n as $o,o as Ho,p as Bo,_ as Go,B as Uo,$ as _o,A as Wo,a0 as Zo,c as qo,a1 as Yo,P as Ko,y as Xo,a2 as Jo,a as Qo,b as er,a3 as nr}from"./connectionPresentation-C_q4Noyw.js";import"./client-6EFRD0gB.js";import{C as G}from"./index-BhpGBZnY.js";import{k as tr,o as Ne,G as or,j as ee,M as tn,i as on,b as rn,a as Te,D as an,d as sn,e as cn,g as rr,C as Pe}from"./connections-DMQz4Kw3.js";import{m as j,c as ar}from"./theme-frt22LyB.js";import{L as ln,T as sr,B as ir,D as cr,A as lr,E as Bt,G as L,a as dr}from"./ConnectionFormStep-DrNDAjua.js";import{L as ur,I as pr,C as mr,a as gr}from"./ConnectionCard-C2nb7zob.js";import{c as Ge}from"./createSvgIcon-ByfhJewD.js";import{a as Gt,O as fr,R as hr,f as dn}from"./index-BXrwOJ9g.js";import{c as br,e as De,f as Cr,g as un,P as xr,b as ne,B as _}from"./Button-qoqN5xvQ.js";import{g as Ut,u as yr,T as A}from"./Typography-USnMzuFJ.js";import{u as Re,i as Ue,l as _e,s as F,a as _t,n as R,g as We,o as W,p as K,A as Ze,B as qe,K as pn,L as vr}from"./createSimplePaletteValueFilter-CvlNngyw.js";import{A as Y,C as Ye}from"./Container-DwRkeloD.js";import{B as T,C as Oe}from"./Box-CWJIOfo9.js";import{I as Sr}from"./IconButton-T-YZe696.js";import{b as jr,L as Wt,A as wr,C as Zt,u as kr,d as Ar,e as Ir}from"./ConnectorDetail-DLdbCdOJ.js";import{C as qt,a as Yt}from"./ConnectorCard-CPtu7Lig.js";import{u as Er}from"./Chip-BT1MOgdz.js";import"./index-M3uX8AIl.js";import"./useThemeProps-2qsqrhXJ.js";import"./Close-BpyWLyqp.js";import"./Stack-L6r6t084.js";import"./ConnectorLogo-D_Yoa9qw.js";function mn(e){return e.substring(2).toLowerCase()}function Tr(e,n){return n.documentElement.clientWidth<e.clientX||n.documentElement.clientHeight<e.clientY}function Pr(e){const{children:n,disableReactTree:o=!1,mouseEvent:r="onClick",onClickAway:i,touchEvent:l="onTouchEnd"}=e,m=p.useRef(!1),d=p.useRef(null),g=p.useRef(!1),C=p.useRef(!1);p.useEffect(()=>(setTimeout(()=>{g.current=!0},0),()=>{g.current=!1}),[]);const s=br(tr(n),d),f=De(a=>{const h=C.current;C.current=!1;const k=Ne(d.current);if(!g.current||!d.current||"clientX"in a&&Tr(a,k))return;if(m.current){m.current=!1;return}let b;a.composedPath?b=a.composedPath().includes(d.current):b=!k.documentElement.contains(a.target)||d.current.contains(a.target),!b&&(o||!h)&&i(a)}),E=a=>h=>{C.current=!0;const k=n.props[a];k&&k(h)},x={ref:s};return l!==!1&&(x[l]=E(l)),p.useEffect(()=>{if(l!==!1){const a=mn(l),h=Ne(d.current),k=()=>{m.current=!0};return h.addEventListener(a,f),h.addEventListener("touchmove",k),()=>{h.removeEventListener(a,f),h.removeEventListener("touchmove",k)}}},[f,l]),r!==!1&&(x[r]=E(r)),p.useEffect(()=>{if(r!==!1){const a=mn(r),h=Ne(d.current);return h.addEventListener(a,f),()=>{h.removeEventListener(a,f)}}},[f,r]),p.cloneElement(n,x)}const ze=typeof Ut({})=="function",Rr=(e,n)=>({WebkitFontSmoothing:"antialiased",MozOsxFontSmoothing:"grayscale",boxSizing:"border-box",WebkitTextSizeAdjust:"100%",...n&&!e.vars&&{colorScheme:e.palette.mode}}),Lr=e=>({color:(e.vars||e).palette.text.primary,...e.typography.body1,backgroundColor:(e.vars||e).palette.background.default,"@media print":{backgroundColor:(e.vars||e).palette.common.white}}),Kt=(e,n=!1)=>{var l,m;const o={};n&&e.colorSchemes&&typeof e.getColorSchemeSelector=="function"&&Object.entries(e.colorSchemes).forEach(([d,g])=>{var s,f;const C=e.getColorSchemeSelector(d);C.startsWith("@")?o[C]={":root":{colorScheme:(s=g.palette)==null?void 0:s.mode}}:o[C.replace(/\s*&/,"")]={colorScheme:(f=g.palette)==null?void 0:f.mode}});let r={html:Rr(e,n),"*, *::before, *::after":{boxSizing:"inherit"},"strong, b":{fontWeight:e.typography.fontWeightBold},body:{margin:0,...Lr(e),"&::backdrop":{backgroundColor:(e.vars||e).palette.background.default}},...o};const i=(m=(l=e.components)==null?void 0:l.MuiCssBaseline)==null?void 0:m.styleOverrides;return i&&(r=[r,i]),r},Ee="mui-ecs",Nr=e=>{const n=Kt(e,!1),o=Array.isArray(n)?n[0]:n;return!e.vars&&o&&(o.html[`:root:has(${Ee})`]={colorScheme:e.palette.mode}),e.colorSchemes&&Object.entries(e.colorSchemes).forEach(([r,i])=>{var m,d;const l=e.getColorSchemeSelector(r);l.startsWith("@")?o[l]={[`:root:not(:has(.${Ee}))`]:{colorScheme:(m=i.palette)==null?void 0:m.mode}}:o[l.replace(/\s*&/,"")]={[`&:not(:has(.${Ee}))`]:{colorScheme:(d=i.palette)==null?void 0:d.mode}}}),n},Mr=Ut(ze?({theme:e,enableColorScheme:n})=>Kt(e,n):({theme:e})=>Nr(e));function Dr(e){const n=Re({props:e,name:"MuiCssBaseline"}),{children:o,enableColorScheme:r=!1}=n;return t.jsxs(p.Fragment,{children:[ze&&t.jsx(Mr,{enableColorScheme:r}),!ze&&!r&&t.jsx("span",{className:Ee,style:{display:"none"}}),o]})}function Or(e){return Ue("MuiLinearProgress",e)}_e("MuiLinearProgress",["root","colorPrimary","colorSecondary","determinate","indeterminate","buffer","query","dashed","dashedColorPrimary","dashedColorSecondary","bar","bar1","bar2","barColorPrimary","barColorSecondary","bar1Indeterminate","bar1Determinate","bar1Buffer","bar2Indeterminate","bar2Buffer"]);const Fe=4,Ve=qe`
  0% {
    left: -35%;
    right: 100%;
  }

  60% {
    left: 100%;
    right: -90%;
  }

  100% {
    left: 100%;
    right: -90%;
  }
`,zr=typeof Ve!="string"?Ze`
        animation: ${Ve} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite;
      `:null,$e=qe`
  0% {
    left: -200%;
    right: 100%;
  }

  60% {
    left: 107%;
    right: -8%;
  }

  100% {
    left: 107%;
    right: -8%;
  }
`,Fr=typeof $e!="string"?Ze`
        animation: ${$e} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite;
      `:null,He=qe`
  0% {
    opacity: 1;
    background-position: 0 -23px;
  }

  60% {
    opacity: 0;
    background-position: 0 -23px;
  }

  100% {
    opacity: 1;
    background-position: -200px -23px;
  }
`,Vr=typeof He!="string"?Ze`
        animation: ${He} 3s infinite linear;
      `:null,$r=e=>{const{classes:n,variant:o,color:r}=e,i={root:["root",`color${R(r)}`,o],dashed:["dashed",`dashedColor${R(r)}`],bar1:["bar","bar1",`barColor${R(r)}`,(o==="indeterminate"||o==="query")&&"bar1Indeterminate",o==="determinate"&&"bar1Determinate",o==="buffer"&&"bar1Buffer"],bar2:["bar","bar2",o!=="buffer"&&`barColor${R(r)}`,o==="buffer"&&`color${R(r)}`,(o==="indeterminate"||o==="query")&&"bar2Indeterminate",o==="buffer"&&"bar2Buffer"]};return We(i,Or,n)},Ke=(e,n)=>e.vars?e.vars.palette.LinearProgress[`${n}Bg`]:e.palette.mode==="light"?e.lighten(e.palette[n].main,.62):e.darken(e.palette[n].main,.5),Hr=F("span",{name:"MuiLinearProgress",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`color${R(o.color)}`],n[o.variant]]}})(W(({theme:e})=>({position:"relative",overflow:"hidden",display:"block",height:4,zIndex:0,"@media print":{colorAdjust:"exact"},variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:Ke(e,n)}})),{props:({ownerState:n})=>n.color==="inherit"&&n.variant!=="buffer",style:{"&::before":{content:'""',position:"absolute",left:0,top:0,right:0,bottom:0,backgroundColor:"currentColor",opacity:.3}}},{props:{variant:"buffer"},style:{backgroundColor:"transparent"}},{props:{variant:"query"},style:{transform:"rotate(180deg)"}}]}))),Br=F("span",{name:"MuiLinearProgress",slot:"Dashed",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.dashed,n[`dashedColor${R(o.color)}`]]}})(W(({theme:e})=>({position:"absolute",marginTop:0,height:"100%",width:"100%",backgroundSize:"10px 10px",backgroundPosition:"0 -23px",variants:[{props:{color:"inherit"},style:{opacity:.3,backgroundImage:"radial-gradient(currentColor 0%, currentColor 16%, transparent 42%)"}},...Object.entries(e.palette).filter(K()).map(([n])=>{const o=Ke(e,n);return{props:{color:n},style:{backgroundImage:`radial-gradient(${o} 0%, ${o} 16%, transparent 42%)`}}})]})),Vr||{animation:`${He} 3s infinite linear`}),Gr=F("span",{name:"MuiLinearProgress",slot:"Bar1",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar1,n[`barColor${R(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar1Indeterminate,o.variant==="determinate"&&n.bar1Determinate,o.variant==="buffer"&&n.bar1Buffer]}})(W(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[{props:{color:"inherit"},style:{backgroundColor:"currentColor"}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:(e.vars||e).palette[n].main}})),{props:{variant:"determinate"},style:{transition:`transform .${Fe}s linear`}},{props:{variant:"buffer"},style:{zIndex:1,transition:`transform .${Fe}s linear`}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:zr||{animation:`${Ve} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite`}}]}))),Ur=F("span",{name:"MuiLinearProgress",slot:"Bar2",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar2,n[`barColor${R(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar2Indeterminate,o.variant==="buffer"&&n.bar2Buffer]}})(W(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{"--LinearProgressBar2-barColor":(e.vars||e).palette[n].main}})),{props:({ownerState:n})=>n.variant!=="buffer"&&n.color!=="inherit",style:{backgroundColor:"var(--LinearProgressBar2-barColor, currentColor)"}},{props:({ownerState:n})=>n.variant!=="buffer"&&n.color==="inherit",style:{backgroundColor:"currentColor"}},{props:{color:"inherit"},style:{opacity:.3}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n,variant:"buffer"},style:{backgroundColor:Ke(e,n),transition:`transform .${Fe}s linear`}})),{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:Fr||{animation:`${$e} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite`}}]}))),_r=p.forwardRef(function(n,o){const r=Re({props:n,name:"MuiLinearProgress"}),{className:i,color:l="primary",value:m,valueBuffer:d,variant:g="indeterminate",...C}=r,s={...r,color:l,variant:g},f=$r(s),E=Er(),x={},a={bar1:{},bar2:{}};if((g==="determinate"||g==="buffer")&&m!==void 0){x["aria-valuenow"]=Math.round(m),x["aria-valuemin"]=0,x["aria-valuemax"]=100;let h=m-100;E&&(h=-h),a.bar1.transform=`translateX(${h}%)`}if(g==="buffer"&&d!==void 0){let h=(d||0)-100;E&&(h=-h),a.bar2.transform=`translateX(${h}%)`}return t.jsxs(Hr,{className:_t(f.root,i),ownerState:s,role:"progressbar",...x,ref:o,...C,children:[g==="buffer"?t.jsx(Br,{className:f.dashed,ownerState:s}):null,t.jsx(Gr,{className:f.bar1,ownerState:s,style:a.bar1}),g==="determinate"?null:t.jsx(Ur,{className:f.bar2,ownerState:s,style:a.bar2})]})});function Wr(e={}){const{autoHideDuration:n=null,disableWindowBlurListener:o=!1,onClose:r,open:i,resumeHideDuration:l}=e,m=Cr();p.useEffect(()=>{if(!i)return;function b(y){y.defaultPrevented||y.key==="Escape"&&(r==null||r(y,"escapeKeyDown"))}return document.addEventListener("keydown",b),()=>{document.removeEventListener("keydown",b)}},[i,r]);const d=De((b,y)=>{r==null||r(b,y)}),g=De(b=>{!r||b==null||m.start(b,()=>{d(null,"timeout")})});p.useEffect(()=>(i&&g(n),m.clear),[i,n,g,m]);const C=b=>{r==null||r(b,"clickaway")},s=m.clear,f=p.useCallback(()=>{n!=null&&g(l??n*.5)},[n,l,g]),E=b=>y=>{const S=b.onBlur;S==null||S(y),f()},x=b=>y=>{const S=b.onFocus;S==null||S(y),s()},a=b=>y=>{const S=b.onMouseEnter;S==null||S(y),s()},h=b=>y=>{const S=b.onMouseLeave;S==null||S(y),f()};return p.useEffect(()=>{if(!o&&i)return window.addEventListener("focus",f),window.addEventListener("blur",s),()=>{window.removeEventListener("focus",f),window.removeEventListener("blur",s)}},[o,i,f,s]),{getRootProps:(b={})=>{const y={...un(e),...un(b)};return{role:"presentation",...b,...y,onBlur:E(y),onFocus:x(y),onMouseEnter:a(y),onMouseLeave:h(y)}},onClickAway:C}}function Zr(e){return Ue("MuiSnackbarContent",e)}_e("MuiSnackbarContent",["root","message","action"]);const qr=e=>{const{classes:n}=e;return We({root:["root"],action:["action"],message:["message"]},Zr,n)},Yr=F(xr,{name:"MuiSnackbarContent",slot:"Root"})(W(({theme:e})=>{const n=e.palette.mode==="light"?.8:.98;return{...e.typography.body2,color:e.vars?e.vars.palette.SnackbarContent.color:e.palette.getContrastText(pn(e.palette.background.default,n)),backgroundColor:e.vars?e.vars.palette.SnackbarContent.bg:pn(e.palette.background.default,n),display:"flex",alignItems:"center",flexWrap:"wrap",padding:"6px 16px",flexGrow:1,[e.breakpoints.up("sm")]:{flexGrow:"initial",minWidth:288}}})),Kr=F("div",{name:"MuiSnackbarContent",slot:"Message"})({padding:"8px 0"}),Xr=F("div",{name:"MuiSnackbarContent",slot:"Action"})({display:"flex",alignItems:"center",marginLeft:"auto",paddingLeft:16,marginRight:-8}),Jr=p.forwardRef(function(n,o){const r=Re({props:n,name:"MuiSnackbarContent"}),{action:i,className:l,message:m,role:d="alert",...g}=r,C=r,s=qr(C);return t.jsxs(Yr,{role:d,elevation:6,className:_t(s.root,l),ownerState:C,ref:o,...g,children:[t.jsx(Kr,{className:s.message,ownerState:C,children:m}),i?t.jsx(Xr,{className:s.action,ownerState:C,children:i}):null]})});function Qr(e){return Ue("MuiSnackbar",e)}_e("MuiSnackbar",["root","anchorOriginTopCenter","anchorOriginBottomCenter","anchorOriginTopRight","anchorOriginBottomRight","anchorOriginTopLeft","anchorOriginBottomLeft"]);const ea=e=>{const{classes:n,anchorOrigin:o}=e,r={root:["root",`anchorOrigin${R(o.vertical)}${R(o.horizontal)}`]};return We(r,Qr,n)},na=F("div",{name:"MuiSnackbar",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`anchorOrigin${R(o.anchorOrigin.vertical)}${R(o.anchorOrigin.horizontal)}`]]}})(W(({theme:e})=>({zIndex:(e.vars||e).zIndex.snackbar,position:"fixed",display:"flex",left:8,right:8,justifyContent:"center",alignItems:"center",variants:[{props:({ownerState:n})=>n.anchorOrigin.vertical==="top",style:{top:8,[e.breakpoints.up("sm")]:{top:24}}},{props:({ownerState:n})=>n.anchorOrigin.vertical!=="top",style:{bottom:8,[e.breakpoints.up("sm")]:{bottom:24}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="left",style:{justifyContent:"flex-start",[e.breakpoints.up("sm")]:{left:24,right:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="right",style:{justifyContent:"flex-end",[e.breakpoints.up("sm")]:{right:24,left:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="center",style:{[e.breakpoints.up("sm")]:{left:"50%",right:"auto",transform:"translateX(-50%)"}}}]}))),ta=p.forwardRef(function(n,o){const r=Re({props:n,name:"MuiSnackbar"}),i=yr(),l={enter:i.transitions.duration.enteringScreen,exit:i.transitions.duration.leavingScreen},{action:m,anchorOrigin:{vertical:d,horizontal:g}={vertical:"bottom",horizontal:"left"},autoHideDuration:C=null,children:s,className:f,ClickAwayListenerProps:E,ContentProps:x,disableWindowBlurListener:a=!1,message:h,onBlur:k,onClose:b,onFocus:y,onMouseEnter:S,onMouseLeave:Z,open:V,resumeHideDuration:J,slots:u={},slotProps:c={},TransitionComponent:w,transitionDuration:M=l,TransitionProps:{onEnter:q,onExited:$,...no}={},...to}=r,H={...r,anchorOrigin:{vertical:d,horizontal:g},autoHideDuration:C,disableWindowBlurListener:a,TransitionComponent:w,transitionDuration:M},oo=ea(H),{getRootProps:ro,onClickAway:ao}=Wr({...H}),[so,Je]=p.useState(!0),io=P=>{Je(!0),$&&$(P)},co=(P,N)=>{Je(!1),q&&q(P,N)},Q={slots:{transition:w,...u},slotProps:{content:x,clickAwayListener:E,transition:no,...c}},[lo,uo]=ne("root",{ref:o,className:[oo.root,f],elementType:na,getSlotProps:ro,externalForwardedProps:{...Q,...to},ownerState:H}),[po,{ownerState:mo,...go}]=ne("clickAwayListener",{elementType:Pr,externalForwardedProps:Q,getSlotProps:P=>({onClickAway:(...N)=>{var Qe;const O=N[0];(Qe=P.onClickAway)==null||Qe.call(P,...N),!(O!=null&&O.defaultMuiPrevented)&&ao(...N)}}),ownerState:H}),[fo,ho]=ne("content",{elementType:Jr,shouldForwardComponentProp:!0,externalForwardedProps:Q,additionalProps:{message:h,action:m},ownerState:H}),[bo,Co]=ne("transition",{elementType:or,externalForwardedProps:Q,getSlotProps:P=>({onEnter:(...N)=>{var O;(O=P.onEnter)==null||O.call(P,...N),co(...N)},onExited:(...N)=>{var O;(O=P.onExited)==null||O.call(P,...N),io(...N)}}),additionalProps:{appear:!0,in:V,timeout:M,direction:d==="top"?"down":"up"},ownerState:H});return!V&&so?null:t.jsx(po,{...go,...u.clickAwayListener&&{ownerState:mo},children:t.jsx(lo,{...uo,children:t.jsx(bo,{...Co,children:s||t.jsx(fo,{...ho})})})})}),oa=Ge(t.jsx("path",{d:"M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2zm10 14.5V20H8v-3.5l4-4zm-4-5-4-4V4h8v3.5z"})),ra=Ge(t.jsx("path",{d:"M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2m6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1zm-2 1H8v-6c0-2.48 1.51-4.5 4-4.5s4 2.02 4 4.5z"})),aa=Ge([t.jsx("path",{d:"M12 5.99 19.53 19H4.47zM12 2 1 21h22z"},"0"),t.jsx("path",{d:"M13 16h-2v2h2zm0-6h-2v5h2z"},"1")]),sa=e=>e.level===Me.ERROR?t.jsx(Bt,{color:"error",fontSize:"small"}):e.level===Me.WARNING?t.jsx(aa,{color:"warning",fontSize:"small"}):t.jsx(pr,{color:"info",fontSize:"small"}),ia=e=>{try{const n=new URL(e,window.location.origin);return n.origin!==window.location.origin?null:`${n.pathname}${n.search}${n.hash}`}catch{return null}},Xt=()=>{const e=Be(),n=Gt(),o=v(xo),[r,i]=p.useState(null),[l,m]=p.useState(null),d=!!r,g=!!l,C=v(yo),s=v(vo),f=v(So),E=v(jo),x=v(wo),a=p.useMemo(()=>s.filter(u=>!u.viewed).map(u=>u.id),[s]);p.useEffect(()=>{o&&f==="idle"&&e(ko())},[o,e,f]);const h=u=>{i(u.currentTarget)},k=()=>{i(null)},b=()=>{k(),e(Io())},y=u=>{m(u.currentTarget),a.length>0&&e(Ao(a))},S=()=>{m(null)},Z=u=>{if(!u.actionUrl||!u.canAction)return;S();const c=ia(u.actionUrl);if(c){n(c);return}window.location.href=u.actionUrl},V=C.length==0?"":C.map((u,c)=>t.jsx(ta,{open:!0,autoHideDuration:6e3,onClose:()=>e(en(c)),anchorOrigin:{vertical:"bottom",horizontal:"center"},children:t.jsx(Y,{onClose:()=>e(en(c)),severity:u.type,sx:{width:"100%"},children:u.message})},u.id)),J=s.length===0?t.jsx(ee,{disabled:!0,children:t.jsx(ln,{primary:"No notifications",primaryTypographyProps:{variant:"body2",color:"text.secondary"}})}):s.map(u=>t.jsxs(ee,{disableRipple:!0,sx:{alignItems:"flex-start",gap:1.5,maxWidth:420,minWidth:{xs:300,sm:380},py:1.5,whiteSpace:"normal"},children:[t.jsx(ur,{sx:{minWidth:32,pt:.25},children:sa(u)}),t.jsx(ln,{primary:u.title,secondary:u.message,primaryTypographyProps:{variant:"subtitle2",fontWeight:u.viewed?500:700},secondaryTypographyProps:{variant:"body2",color:"text.secondary",sx:{mt:.5}}}),u.canAction&&u.actionUrl&&t.jsx(_,{size:"small",variant:"outlined",onClick:c=>{c.stopPropagation(),Z(u)},sx:{flexShrink:0,mt:.25},children:"Open"})]},u.id));return t.jsxs(T,{sx:{display:"flex",flexDirection:"column",minHeight:"100vh",bgcolor:"background.default"},children:[o&&t.jsxs(Ye,{maxWidth:"lg",sx:{display:"flex",justifyContent:"flex-end",alignItems:"center",gap:1,pt:{xs:1,sm:2}},children:[t.jsx(sr,{title:"Open notifications",children:t.jsx(Sr,{id:"notifications-button",color:"inherit",size:"small",onClick:y,"aria-controls":g?"notifications-menu":void 0,"aria-haspopup":"true","aria-expanded":g?"true":void 0,"aria-label":"Open notifications",sx:{color:"text.secondary"},children:t.jsx(ir,{badgeContent:x,color:"warning",invisible:x===0,children:t.jsx(ra,{fontSize:"small"})})})}),t.jsxs(tn,{id:"notifications-menu",anchorEl:l,open:g,onClose:S,MenuListProps:{"aria-labelledby":"notifications-button"},PaperProps:{sx:{mt:1,maxHeight:480,borderRadius:j.radius.panel}},children:[t.jsxs(T,{sx:{px:2,py:1.25},children:[t.jsx(A,{variant:"subtitle1",component:"p",sx:{fontWeight:700},children:"Notifications"}),f==="failed"&&t.jsx(A,{variant:"body2",color:"error",children:E??"Could not load notifications"})]}),t.jsx(cr,{}),J]}),t.jsx(_,{id:"account-button",onClick:h,color:"inherit",size:"small",endIcon:t.jsx(lr,{alt:o,src:"/assets/avatar.png",sx:{width:28,height:28,fontSize:14}}),"aria-controls":d?"account-menu":void 0,"aria-haspopup":"true","aria-expanded":d?"true":void 0,sx:{color:"text.secondary",minWidth:0,textTransform:"none"},children:t.jsx(A,{variant:"body2",component:"span",noWrap:!0,sx:{display:{xs:"none",sm:"inline"},maxWidth:260},children:o})}),t.jsxs(tn,{id:"account-menu",anchorEl:r,open:d,onClose:k,MenuListProps:{"aria-labelledby":"account-button"},children:[t.jsx(ee,{disabled:!0,children:t.jsx(A,{variant:"body2",children:o})}),t.jsx(ee,{onClick:b,children:"Logout"})]})]}),t.jsxs(T,{component:"main",sx:{flexGrow:1},children:[t.jsx(fr,{}),V]})]})};Xt.__docgenInfo={description:"Layout component for the application",methods:[],displayName:"Layout"};const Jt=()=>{const e=Be(),n=Gt(),o=v(Ft),r=v(Vt),i=v($t),{cancelForm:l,connect:m,currentFormStep:d,formSubmitError:g,isConnecting:C,isSubmittingForm:s,submitForm:f}=jr();p.useEffect(()=>{r==="idle"&&e(Ht())},[r,e]);const E=p.useCallback(a=>{n(`/connectors/${encodeURIComponent(a)}`)},[n]);let x;return r==="loading"?x=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(a=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Yt,{})},a))}):r==="failed"?x=t.jsx(Y,{severity:"error",children:i}):o.length===0?x=t.jsx(T,{sx:{textAlign:"center",py:j.spacing.pageY},children:t.jsx(A,{variant:"h6",color:"text.secondary",children:"No connectors available"})}):x=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:o.map(a=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(qt,{connector:a,onConnect:m,onDetails:E,isConnecting:C})},a.id))}),t.jsxs(Ye,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(A,{variant:"h4",component:"h1",children:"Available Connectors"}),t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:j.spacing.headerGap},children:[C&&t.jsxs(T,{sx:{display:"flex",alignItems:"center"},children:[t.jsx(Oe,{size:24,sx:{mr:1}}),t.jsx(A,{variant:"body2",color:"text.secondary",children:"Connecting..."})]}),t.jsx(_,{component:Wt,to:"/connections",startIcon:t.jsx(wr,{}),sx:{alignSelf:{xs:"flex-start",sm:"center"}},children:"Back to Connections"})]})]}),x,t.jsx(Zt,{currentFormStep:d,formSubmitError:g,isSubmittingForm:s,onCancel:l,onSubmit:f})]})};Jt.__docgenInfo={description:"Component to display a list of available connectors",methods:[],displayName:"ConnectorList"};const Qt=()=>{const e=Be(),[n,o]=kr(),r=v(Eo),i=v(To),l=v(Po),m=v(Ft),d=v(Vt),g=v($t),C=v(Ro),s=v(Lo),f=v(No),E=v(Mo),x=v(Do),a=v(Oo),h=v(zo),k=v(Fo);p.useEffect(()=>{i==="idle"&&e(B()),d==="idle"&&e(Ht())},[i,d,e]),p.useEffect(()=>{const c=n.get("setup"),w=n.get("connectionId");c==="pending"&&w&&(e(nn(w)),n.delete("setup"),n.delete("connectionId"),o(n,{replace:!0}))},[n,o,e]),p.useEffect(()=>{if(!x)return;const c=window.setInterval(()=>{e(nn(x))},2e3);return()=>window.clearInterval(c)},[x,e]),p.useEffect(()=>{if(!k)return;e(B());const c=window.setTimeout(()=>{e(Vo())},3500);return()=>window.clearTimeout(c)},[e,k]);const b=p.useCallback((c,w)=>{const M=(s==null?void 0:s.stepId)??"";e($o({connectionId:c,stepId:M,data:w,returnToUrl:window.location.href})).then(q=>{if(q.meta.requestStatus==="fulfilled"){const $=q.payload;on($)?window.location.href=$.redirectUrl:rn($)?e(B()):e(B())}})},[e,s]),y=p.useCallback(()=>{const c=s==null?void 0:s.connectionId,w=c?r.find(M=>M.id===c):void 0;w&&w.state===Te.CONFIGURED&&e(Ho(w.id)),e(Bo())},[e,s,r]),S=p.useCallback(()=>{a&&e(Go({connectionId:a.connectionId,returnToUrl:window.location.href})).then(c=>{if(c.meta.requestStatus==="fulfilled"){const w=c.payload;w.type==="redirect"&&w.redirectUrl&&(window.location.href=w.redirectUrl)}})},[e,a]),Z=p.useCallback(()=>{a&&e(Uo(a.connectionId)).then(()=>{e(_o()),e(B())})},[e,a]),V=p.useCallback(c=>{e(Wo({connectorId:c,returnToUrl:`${window.location.origin}/connections`})).then(w=>{if(w.meta.requestStatus==="fulfilled"){const M=w.payload;on(M)?window.location.href=M.redirectUrl:rn(M)&&e(B())}})},[e]),J=()=>d==="loading"||d==="idle"?t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Yt,{})},`connector-skeleton-${c}`))}):d==="failed"?t.jsx(Y,{severity:"error",children:g}):m.length===0?t.jsx(T,{sx:{py:3},children:t.jsx(A,{color:"text.secondary",children:"No connectors are available right now."})}):t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:m.map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(qt,{connector:c,onConnect:V,isConnecting:C})},c.id))});let u;return i==="loading"?u=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(gr,{})},`connection-skeleton-${c}`))}):i==="failed"?u=t.jsx(Y,{severity:"error",children:l}):r.length===0?u=t.jsxs(t.Fragment,{children:[t.jsxs(T,{sx:{border:1,borderColor:j.card.borderColor,borderRadius:j.radius.panel,bgcolor:j.card.surface,mb:j.spacing.sectionGap,p:j.spacing.panelPadding},children:[t.jsx(A,{variant:"h5",component:"h2",gutterBottom:!0,children:"Connect your first application"}),t.jsx(A,{color:"text.secondary",sx:{maxWidth:680},children:"Choose a connector below to create a connection. Once connected, it will appear here for ongoing setup, health, and management."}),C&&t.jsxs(T,{sx:{display:"flex",alignItems:"center",mt:3},children:[t.jsx(Oe,{size:24,sx:{mr:1}}),t.jsx(A,{variant:"body2",color:"text.secondary",children:"Starting connection..."})]})]}),t.jsxs(T,{children:[t.jsx(A,{variant:"h6",component:"h2",sx:{mb:2},children:"Available connectors"}),J()]})]}):u=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:r.map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(mr,{connection:c,highlightNew:c.id===k})},c.id))}),t.jsxs(Ye,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(A,{variant:"h4",component:"h1",children:"Your Connections"}),r.length>0&&t.jsx(_,{variant:"contained",color:"primary",startIcon:t.jsx(dr,{}),component:Wt,to:"/connectors",children:"Connect More"})]}),u,t.jsx(Zt,{currentFormStep:s,formSubmitError:E,isSubmittingForm:f,onCancel:y,onSubmit:b}),t.jsxs(an,{open:x!==null,maxWidth:"xs",fullWidth:!0,children:[t.jsx(sn,{sx:{pb:1},children:"Verifying connection"}),t.jsx(cn,{dividers:!0,children:t.jsxs(T,{sx:{display:"flex",flexDirection:"column",alignItems:"center",gap:j.spacing.headerGap,py:3},children:[t.jsx(oa,{color:"primary",sx:{fontSize:40}}),t.jsxs(T,{sx:{textAlign:"center"},children:[t.jsx(A,{variant:"subtitle1",component:"p",children:"Checking credentials"}),t.jsx(A,{variant:"body2",color:"text.secondary",children:"AuthProxy is confirming that this connection can reach the provider."})]}),t.jsx(_r,{sx:{width:"100%"}})]})})]}),t.jsxs(an,{open:a!==null,onClose:Z,maxWidth:"sm",fullWidth:!0,children:[t.jsx(sn,{sx:{pb:1},children:t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:1},children:[t.jsx(Bt,{color:"error"}),t.jsx(A,{variant:"h6",component:"span",children:"Connection verification failed"})]})}),t.jsxs(cn,{dividers:!0,children:[t.jsxs(Y,{severity:"error",sx:{mb:2},children:[t.jsx(Ar,{children:"Provider check failed"}),(a==null?void 0:a.message)??"Verification failed"]}),t.jsx(A,{variant:"body2",color:"text.secondary",children:a!=null&&a.canRetry?"Retry setup to run verification again. Cancel setup deletes this unfinished connection.":"Cancel setup to delete this unfinished connection, then start again from the connector."})]}),t.jsxs(rr,{children:[t.jsx(_,{onClick:Z,disabled:h,children:"Cancel setup"}),(a==null?void 0:a.canRetry)&&t.jsx(_,{onClick:S,disabled:h,variant:"contained",startIcon:h?t.jsx(Oe,{size:16}):void 0,children:h?"Retrying setup...":"Retry setup"})]})]})]})};Qt.__docgenInfo={description:"Component to display a list of connections",methods:[],displayName:"ConnectionList"};const U=(e,n,o="#ffffff")=>{const r=e.split(/\s+/).map(l=>l[0]).join("").slice(0,2).toUpperCase(),i=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${n}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${o}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${r}</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(i)}`},D=[{id:"google-drive",namespace:"root",version:1,state:G.ACTIVE,displayName:"Google Drive",description:"Have the agent track your work in Google Drive.",highlight:"Have the agent track your work in Google Drive.",logo:U("Google Drive","#188038"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"greenhouse",namespace:"root",version:1,state:G.ACTIVE,displayName:"Greenhouse",description:"This integration pushes candidates to greenhouse.",highlight:"This integration pushes candidates to greenhouse.",logo:U("Greenhouse","#24a47f"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"google-calendar",namespace:"root",version:1,state:G.ACTIVE,displayName:"Google Calendar",description:`Google Calendar lets agents coordinate scheduling work without needing direct access to your primary app.

![Calendar workflow preview](/calendar-workflow-preview.svg)

### What agents can do

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create and update events | Yes |
| Read attendee responses | Yes |
| Manage private event details | No |

Use this connector when the assistant should propose meeting times, create holds, or keep follow-up work attached to calendar events.`,highlight:"Coordinate meetings, availability, and follow-up from Google Calendar.",logo:U("Google Calendar","#1a73e8"),hasConfigure:!0,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"gmail",namespace:"root",version:1,state:G.ACTIVE,displayName:"GMail",description:"Have the agent respond to your emails without you needing to be involved. Like magic.",highlight:"Have the agent respond to your emails without you needing to be involved. Like magic.",logo:U("GMail","#d93025"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"pipedrive",namespace:"root",version:1,state:G.ACTIVE,displayName:"pipedrive",description:"Allow our agent to handle your sales support.",highlight:"Allow our agent to handle your sales support.",logo:U("pipedrive","#017a5e"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"asana",namespace:"root",version:1,state:G.ACTIVE,displayName:"Asana",description:"Allow our agent organize your work.",highlight:"Allow our agent organize your work.",logo:U("Asana","#f06a6a"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"}],z=(e,n={})=>({id:`cxn_${e.id}`,namespace:"root",connector:e,state:Te.CONFIGURED,healthState:Pe.HEALTHY,createdAt:"2024-04-01T12:00:00Z",updatedAt:"2024-04-01T12:00:00Z",...n}),ca=[z(D[0]),z(D[2],{healthState:Pe.UNHEALTHY}),z(D[5],{state:Te.SETUP}),z(D[4],{state:Te.DISABLED})],Xe={connectionId:"cxn_google-calendar",stepId:"select-calendar",stepTitle:"Select a Calendar",stepDescription:"Choose which Google Calendar the agent should manage.",currentStep:0,totalSteps:2,jsonSchema:{type:"object",required:["calendar_id"],properties:{calendar_id:{type:"string",title:"Calendar",enum:["primary","product","support"]}}},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar_id"}]}},I={items:ca,status:"succeeded",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null,currentFormStep:null,submittingForm:!1,formSubmitError:null,verifyingConnectionId:null,verifyError:null,retryingConnection:!1,recentlyCompletedConnectionId:null},eo={items:[],status:"succeeded",error:null,markingViewed:!1};function la({route:e,colorMode:n="light",connectorsState:o={items:D,status:"succeeded",error:null},connectionsState:r=I,notificationsState:i=eo}){const l=p.useMemo(()=>ar(n),[n]),m=qo({reducer:Yo({auth:nr,connectors:er,connections:Qo,notifications:Jo,toasts:Xo}),preloadedState:{auth:{actorId:"actor_storybook",status:"authenticated"},connectors:o,connections:r,notifications:i,toasts:{items:[]}}});return t.jsx(Ko,{store:m,children:t.jsxs(vr,{theme:l,children:[t.jsx(Dr,{enableColorScheme:!0}),t.jsx(hr,{children:t.jsx(dn,{element:t.jsx(Xt,{}),children:t.jsx(dn,{path:"*",element:e==="/connectors"?t.jsx(Jt,{}):e==="/connector-detail"?t.jsx(Ir,{connectorId:"google-calendar"}):t.jsx(Qt,{})})})})]})})}const Da={title:"Pages/Marketplace",component:la,parameters:{layout:"fullscreen"}},Le={viewport:{defaultViewport:"marketplaceMobile"}},X={viewport:{defaultViewport:"marketplaceTablet"}},te={args:{route:"/connectors"}},oe={args:{route:"/connectors",colorMode:"dark"}},re={args:{route:"/connector-detail"}},ae={args:{route:"/connector-detail"},parameters:Le},se={args:{route:"/connectors",connectorsState:{items:[],status:"loading",error:null}}},ie={args:{route:"/connections"}},ce={args:{route:"/connections",colorMode:"dark"}},le={args:{route:"/connections"},parameters:Le},de={args:{route:"/connections"},parameters:X},ue={args:{route:"/connections",connectionsState:{...I,items:[z(D[2],{healthState:Pe.UNHEALTHY})]}}},pe={args:{route:"/connections",connectionsState:{...I,items:[z(D[2],{healthState:Pe.UNHEALTHY}),z(D[5],{setupStepId:"select-workspace"})]},notificationsState:{...eo,items:[{id:"ntf_reauth",key:"connection:cxn_google-calendar:auth_required",level:Me.WARNING,state:Zo.ACTIVE,resourceType:"connection",resourceId:"cxn_google-calendar",namespace:"root",title:"Connection requires re-authentication",message:"Reconnect this connection to continue using it.",actionUrl:"/connections/cxn_google-calendar?action=reauth",canAction:!0,viewed:!1,createdAt:"2026-07-12T12:00:00Z",updatedAt:"2026-07-12T12:00:00Z"}]}}},me={args:{route:"/connections",connectionsState:{...I,items:[z(D[2]),z(D[0])]}}},ge={args:{route:"/connections",connectionsState:{...I,items:[]}}},fe={args:{route:"/connections",connectionsState:{...I,items:[]}},parameters:Le},he={args:{route:"/connections",connectionsState:{...I,items:[]}},parameters:X},be={args:{route:"/connections",connectorsState:{items:[],status:"loading",error:null},connectionsState:{...I,items:[]}}},Ce={args:{route:"/connectors"},parameters:Le},xe={args:{route:"/connectors"},parameters:X},ye={args:{route:"/connections",connectionsState:{...I,currentFormStep:Xe}}},ve={args:{route:"/connections",connectionsState:{...I,currentFormStep:Xe}},parameters:X},Se={args:{route:"/connections",connectionsState:{...I,currentFormStep:Xe,submittingForm:!0}}},je={args:{route:"/connections",connectionsState:{...I,verifyingConnectionId:"cxn_google-calendar"}}},we={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}}},ke={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}},parameters:X},Ae={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0},retryingConnection:!0}}},Ie={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"The provider rejected this setup and it cannot be retried.",canRetry:!1}}}};var gn,fn,hn;te.parameters={...te.parameters,docs:{...(gn=te.parameters)==null?void 0:gn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  }
}`,...(hn=(fn=te.parameters)==null?void 0:fn.docs)==null?void 0:hn.source}}};var bn,Cn,xn;oe.parameters={...oe.parameters,docs:{...(bn=oe.parameters)==null?void 0:bn.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    colorMode: 'dark'
  }
}`,...(xn=(Cn=oe.parameters)==null?void 0:Cn.docs)==null?void 0:xn.source}}};var yn,vn,Sn;re.parameters={...re.parameters,docs:{...(yn=re.parameters)==null?void 0:yn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  }
}`,...(Sn=(vn=re.parameters)==null?void 0:vn.docs)==null?void 0:Sn.source}}};var jn,wn,kn;ae.parameters={...ae.parameters,docs:{...(jn=ae.parameters)==null?void 0:jn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  },
  parameters: mobileViewport
}`,...(kn=(wn=ae.parameters)==null?void 0:wn.docs)==null?void 0:kn.source}}};var An,In,En;se.parameters={...se.parameters,docs:{...(An=se.parameters)==null?void 0:An.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    }
  }
}`,...(En=(In=se.parameters)==null?void 0:In.docs)==null?void 0:En.source}}};var Tn,Pn,Rn;ie.parameters={...ie.parameters,docs:{...(Tn=ie.parameters)==null?void 0:Tn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  }
}`,...(Rn=(Pn=ie.parameters)==null?void 0:Pn.docs)==null?void 0:Rn.source}}};var Ln,Nn,Mn;ce.parameters={...ce.parameters,docs:{...(Ln=ce.parameters)==null?void 0:Ln.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    colorMode: 'dark'
  }
}`,...(Mn=(Nn=ce.parameters)==null?void 0:Nn.docs)==null?void 0:Mn.source}}};var Dn,On,zn;le.parameters={...le.parameters,docs:{...(Dn=le.parameters)==null?void 0:Dn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: mobileViewport
}`,...(zn=(On=le.parameters)==null?void 0:On.docs)==null?void 0:zn.source}}};var Fn,Vn,$n;de.parameters={...de.parameters,docs:{...(Fn=de.parameters)==null?void 0:Fn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: tabletViewport
}`,...($n=(Vn=de.parameters)==null?void 0:Vn.docs)==null?void 0:$n.source}}};var Hn,Bn,Gn;ue.parameters={...ue.parameters,docs:{...(Hn=ue.parameters)==null?void 0:Hn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        healthState: ConnectionHealthState.UNHEALTHY
      })]
    }
  }
}`,...(Gn=(Bn=ue.parameters)==null?void 0:Bn.docs)==null?void 0:Gn.source}}};var Un,_n,Wn;pe.parameters={...pe.parameters,docs:{...(Un=pe.parameters)==null?void 0:Un.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        healthState: ConnectionHealthState.UNHEALTHY
      }), connectionFor(connectors[5], {
        setupStepId: 'select-workspace'
      })]
    },
    notificationsState: {
      ...baseNotificationsState,
      items: [{
        id: 'ntf_reauth',
        key: 'connection:cxn_google-calendar:auth_required',
        level: NotificationLevel.WARNING,
        state: NotificationState.ACTIVE,
        resourceType: 'connection',
        resourceId: 'cxn_google-calendar',
        namespace: 'root',
        title: 'Connection requires re-authentication',
        message: 'Reconnect this connection to continue using it.',
        actionUrl: '/connections/cxn_google-calendar?action=reauth',
        canAction: true,
        viewed: false,
        createdAt: '2026-07-12T12:00:00Z',
        updatedAt: '2026-07-12T12:00:00Z'
      }]
    }
  }
}`,...(Wn=(_n=pe.parameters)==null?void 0:_n.docs)==null?void 0:Wn.source}}};var Zn,qn,Yn;me.parameters={...me.parameters,docs:{...(Zn=me.parameters)==null?void 0:Zn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2]), connectionFor(connectors[0])]
    }
  }
}`,...(Yn=(qn=me.parameters)==null?void 0:qn.docs)==null?void 0:Yn.source}}};var Kn,Xn,Jn;ge.parameters={...ge.parameters,docs:{...(Kn=ge.parameters)==null?void 0:Kn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Jn=(Xn=ge.parameters)==null?void 0:Xn.docs)==null?void 0:Jn.source}}};var Qn,et,nt;fe.parameters={...fe.parameters,docs:{...(Qn=fe.parameters)==null?void 0:Qn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: mobileViewport
}`,...(nt=(et=fe.parameters)==null?void 0:et.docs)==null?void 0:nt.source}}};var tt,ot,rt;he.parameters={...he.parameters,docs:{...(tt=he.parameters)==null?void 0:tt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: tabletViewport
}`,...(rt=(ot=he.parameters)==null?void 0:ot.docs)==null?void 0:rt.source}}};var at,st,it;be.parameters={...be.parameters,docs:{...(at=be.parameters)==null?void 0:at.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    },
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(it=(st=be.parameters)==null?void 0:st.docs)==null?void 0:it.source}}};var ct,lt,dt;Ce.parameters={...Ce.parameters,docs:{...(ct=Ce.parameters)==null?void 0:ct.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: mobileViewport
}`,...(dt=(lt=Ce.parameters)==null?void 0:lt.docs)==null?void 0:dt.source}}};var ut,pt,mt;xe.parameters={...xe.parameters,docs:{...(ut=xe.parameters)==null?void 0:ut.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: tabletViewport
}`,...(mt=(pt=xe.parameters)==null?void 0:pt.docs)==null?void 0:mt.source}}};var gt,ft,ht;ye.parameters={...ye.parameters,docs:{...(gt=ye.parameters)==null?void 0:gt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  }
}`,...(ht=(ft=ye.parameters)==null?void 0:ft.docs)==null?void 0:ht.source}}};var bt,Ct,xt;ve.parameters={...ve.parameters,docs:{...(bt=ve.parameters)==null?void 0:bt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  },
  parameters: tabletViewport
}`,...(xt=(Ct=ve.parameters)==null?void 0:Ct.docs)==null?void 0:xt.source}}};var yt,vt,St;Se.parameters={...Se.parameters,docs:{...(yt=Se.parameters)==null?void 0:yt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true
    }
  }
}`,...(St=(vt=Se.parameters)==null?void 0:vt.docs)==null?void 0:St.source}}};var jt,wt,kt;je.parameters={...je.parameters,docs:{...(jt=je.parameters)==null?void 0:jt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar'
    }
  }
}`,...(kt=(wt=je.parameters)==null?void 0:wt.docs)==null?void 0:kt.source}}};var At,It,Et;we.parameters={...we.parameters,docs:{...(At=we.parameters)==null?void 0:At.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      }
    }
  }
}`,...(Et=(It=we.parameters)==null?void 0:It.docs)==null?void 0:Et.source}}};var Tt,Pt,Rt;ke.parameters={...ke.parameters,docs:{...(Tt=ke.parameters)==null?void 0:Tt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      }
    }
  },
  parameters: tabletViewport
}`,...(Rt=(Pt=ke.parameters)==null?void 0:Pt.docs)==null?void 0:Rt.source}}};var Lt,Nt,Mt;Ae.parameters={...Ae.parameters,docs:{...(Lt=Ae.parameters)==null?void 0:Lt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      },
      retryingConnection: true
    }
  }
}`,...(Mt=(Nt=Ae.parameters)==null?void 0:Nt.docs)==null?void 0:Mt.source}}};var Dt,Ot,zt;Ie.parameters={...Ie.parameters,docs:{...(Dt=Ie.parameters)==null?void 0:Dt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'The provider rejected this setup and it cannot be retried.',
        canRetry: false
      }
    }
  }
}`,...(zt=(Ot=Ie.parameters)==null?void 0:Ot.docs)==null?void 0:zt.source}}};const Oa=["AvailableConnectors","AvailableConnectorsDark","ConnectorOverview","ConnectorOverviewMobile","AvailableConnectorsLoading","ConnectionsPopulated","ConnectionsPopulatedDark","ConnectionsPopulatedMobile","ConnectionsPopulatedTablet","ConnectionsNeedsAttention","ConnectionsWithNotifications","ConnectionsHealthyActions","ConnectionsEmpty","ConnectionsEmptyMobile","ConnectionsEmptyTablet","ConnectionsEmptyLoadingConnectors","AvailableConnectorsMobile","AvailableConnectorsTablet","ConnectionSetupDialog","ConnectionSetupDialogTablet","ConnectionSetupSubmitting","VerifyingConnectionDialog","VerificationFailedDialog","VerificationFailedDialogTablet","VerificationRetryingDialog","VerificationFailedNoRetryDialog"];export{te as AvailableConnectors,oe as AvailableConnectorsDark,se as AvailableConnectorsLoading,Ce as AvailableConnectorsMobile,xe as AvailableConnectorsTablet,ye as ConnectionSetupDialog,ve as ConnectionSetupDialogTablet,Se as ConnectionSetupSubmitting,ge as ConnectionsEmpty,be as ConnectionsEmptyLoadingConnectors,fe as ConnectionsEmptyMobile,he as ConnectionsEmptyTablet,me as ConnectionsHealthyActions,ue as ConnectionsNeedsAttention,ie as ConnectionsPopulated,ce as ConnectionsPopulatedDark,le as ConnectionsPopulatedMobile,de as ConnectionsPopulatedTablet,pe as ConnectionsWithNotifications,re as ConnectorOverview,ae as ConnectorOverviewMobile,we as VerificationFailedDialog,ke as VerificationFailedDialogTablet,Ie as VerificationFailedNoRetryDialog,Ae as VerificationRetryingDialog,je as VerifyingConnectionDialog,Oa as __namedExportsOrder,Da as default};
