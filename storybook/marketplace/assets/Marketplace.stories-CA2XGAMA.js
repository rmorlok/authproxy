import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{u as $e,d as v,H as uo,I as po,J as mo,K as go,L as fo,M as ho,N as bo,O as Je,Q as Le,T as Co,U as xo,C as Rt,E as Pt,F as Lt,G as Nt,s as yo,e as vo,f as So,z as jo,g as wo,h as ko,i as Ao,V as Io,W as Eo,X as To,Y as Ro,j as B,l as Qe,Z as Po,n as Lo,o as No,p as Mo,_ as Do,B as Oo,$ as zo,A as Fo,a0 as Vo,c as $o,a1 as Ho,P as Bo,y as Go,a2 as Uo,a as _o,b as Wo,a3 as Zo}from"./connectionPresentation-C_q4Noyw.js";import"./client-6EFRD0gB.js";import{C as G}from"./index-BhpGBZnY.js";import{k as qo,o as Pe,G as Yo,j as ee,M as en,i as nn,b as tn,a as Ie,D as on,d as rn,e as sn,g as Ko,C as Ee}from"./connections-DMQz4Kw3.js";import{m as j,t as Xo}from"./theme-CKuQCuBO.js";import{r as p}from"./index-yIsmwZOr.js";import{L as an,T as Jo,B as Qo,D as er,A as nr,E as Mt,G as L,a as tr}from"./ConnectionFormStep-Em9wryDt.js";import{L as or,I as rr,C as sr,a as ar}from"./ConnectionCard-Cm6KZdsy.js";import{c as He}from"./createSvgIcon-ByfhJewD.js";import{a as Dt,O as ir,R as cr,f as cn}from"./index-BXrwOJ9g.js";import{c as lr,e as Ne,f as dr,g as ln,P as ur,b as ne,B as _}from"./Button-qoqN5xvQ.js";import{g as Ot,u as pr,T as A}from"./Typography-USnMzuFJ.js";import{u as Te,i as Be,l as Ge,s as F,a as zt,n as P,g as Ue,o as W,p as K,A as _e,B as We,K as dn,L as mr}from"./createSimplePaletteValueFilter-CvlNngyw.js";import{A as Y,C as Ze}from"./Container-DwRkeloD.js";import{B as T,C as Me}from"./Box-CWJIOfo9.js";import{I as gr}from"./IconButton-T-YZe696.js";import{b as fr,L as Ft,A as hr,C as Vt,u as br,d as Cr,e as xr}from"./ConnectorDetail-DigDVHB7.js";import{C as $t,a as Ht}from"./ConnectorCard-CcQUPQCQ.js";import{u as yr}from"./Chip-BT1MOgdz.js";import"./index-M3uX8AIl.js";import"./useThemeProps-2qsqrhXJ.js";import"./Close-BpyWLyqp.js";import"./Stack-L6r6t084.js";import"./ConnectorLogo-P4ORXqmW.js";function un(e){return e.substring(2).toLowerCase()}function vr(e,n){return n.documentElement.clientWidth<e.clientX||n.documentElement.clientHeight<e.clientY}function Sr(e){const{children:n,disableReactTree:o=!1,mouseEvent:r="onClick",onClickAway:i,touchEvent:u="onTouchEnd"}=e,m=p.useRef(!1),l=p.useRef(null),g=p.useRef(!1),C=p.useRef(!1);p.useEffect(()=>(setTimeout(()=>{g.current=!0},0),()=>{g.current=!1}),[]);const a=lr(qo(n),l),f=Ne(s=>{const h=C.current;C.current=!1;const k=Pe(l.current);if(!g.current||!l.current||"clientX"in s&&vr(s,k))return;if(m.current){m.current=!1;return}let b;s.composedPath?b=s.composedPath().includes(l.current):b=!k.documentElement.contains(s.target)||l.current.contains(s.target),!b&&(o||!h)&&i(s)}),E=s=>h=>{C.current=!0;const k=n.props[s];k&&k(h)},x={ref:a};return u!==!1&&(x[u]=E(u)),p.useEffect(()=>{if(u!==!1){const s=un(u),h=Pe(l.current),k=()=>{m.current=!0};return h.addEventListener(s,f),h.addEventListener("touchmove",k),()=>{h.removeEventListener(s,f),h.removeEventListener("touchmove",k)}}},[f,u]),r!==!1&&(x[r]=E(r)),p.useEffect(()=>{if(r!==!1){const s=un(r),h=Pe(l.current);return h.addEventListener(s,f),()=>{h.removeEventListener(s,f)}}},[f,r]),p.cloneElement(n,x)}const De=typeof Ot({})=="function",jr=(e,n)=>({WebkitFontSmoothing:"antialiased",MozOsxFontSmoothing:"grayscale",boxSizing:"border-box",WebkitTextSizeAdjust:"100%",...n&&!e.vars&&{colorScheme:e.palette.mode}}),wr=e=>({color:(e.vars||e).palette.text.primary,...e.typography.body1,backgroundColor:(e.vars||e).palette.background.default,"@media print":{backgroundColor:(e.vars||e).palette.common.white}}),Bt=(e,n=!1)=>{var u,m;const o={};n&&e.colorSchemes&&typeof e.getColorSchemeSelector=="function"&&Object.entries(e.colorSchemes).forEach(([l,g])=>{var a,f;const C=e.getColorSchemeSelector(l);C.startsWith("@")?o[C]={":root":{colorScheme:(a=g.palette)==null?void 0:a.mode}}:o[C.replace(/\s*&/,"")]={colorScheme:(f=g.palette)==null?void 0:f.mode}});let r={html:jr(e,n),"*, *::before, *::after":{boxSizing:"inherit"},"strong, b":{fontWeight:e.typography.fontWeightBold},body:{margin:0,...wr(e),"&::backdrop":{backgroundColor:(e.vars||e).palette.background.default}},...o};const i=(m=(u=e.components)==null?void 0:u.MuiCssBaseline)==null?void 0:m.styleOverrides;return i&&(r=[r,i]),r},Ae="mui-ecs",kr=e=>{const n=Bt(e,!1),o=Array.isArray(n)?n[0]:n;return!e.vars&&o&&(o.html[`:root:has(${Ae})`]={colorScheme:e.palette.mode}),e.colorSchemes&&Object.entries(e.colorSchemes).forEach(([r,i])=>{var m,l;const u=e.getColorSchemeSelector(r);u.startsWith("@")?o[u]={[`:root:not(:has(.${Ae}))`]:{colorScheme:(m=i.palette)==null?void 0:m.mode}}:o[u.replace(/\s*&/,"")]={[`&:not(:has(.${Ae}))`]:{colorScheme:(l=i.palette)==null?void 0:l.mode}}}),n},Ar=Ot(De?({theme:e,enableColorScheme:n})=>Bt(e,n):({theme:e})=>kr(e));function Ir(e){const n=Te({props:e,name:"MuiCssBaseline"}),{children:o,enableColorScheme:r=!1}=n;return t.jsxs(p.Fragment,{children:[De&&t.jsx(Ar,{enableColorScheme:r}),!De&&!r&&t.jsx("span",{className:Ae,style:{display:"none"}}),o]})}function Er(e){return Be("MuiLinearProgress",e)}Ge("MuiLinearProgress",["root","colorPrimary","colorSecondary","determinate","indeterminate","buffer","query","dashed","dashedColorPrimary","dashedColorSecondary","bar","bar1","bar2","barColorPrimary","barColorSecondary","bar1Indeterminate","bar1Determinate","bar1Buffer","bar2Indeterminate","bar2Buffer"]);const Oe=4,ze=We`
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
`,Tr=typeof ze!="string"?_e`
        animation: ${ze} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite;
      `:null,Fe=We`
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
`,Rr=typeof Fe!="string"?_e`
        animation: ${Fe} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite;
      `:null,Ve=We`
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
`,Pr=typeof Ve!="string"?_e`
        animation: ${Ve} 3s infinite linear;
      `:null,Lr=e=>{const{classes:n,variant:o,color:r}=e,i={root:["root",`color${P(r)}`,o],dashed:["dashed",`dashedColor${P(r)}`],bar1:["bar","bar1",`barColor${P(r)}`,(o==="indeterminate"||o==="query")&&"bar1Indeterminate",o==="determinate"&&"bar1Determinate",o==="buffer"&&"bar1Buffer"],bar2:["bar","bar2",o!=="buffer"&&`barColor${P(r)}`,o==="buffer"&&`color${P(r)}`,(o==="indeterminate"||o==="query")&&"bar2Indeterminate",o==="buffer"&&"bar2Buffer"]};return Ue(i,Er,n)},qe=(e,n)=>e.vars?e.vars.palette.LinearProgress[`${n}Bg`]:e.palette.mode==="light"?e.lighten(e.palette[n].main,.62):e.darken(e.palette[n].main,.5),Nr=F("span",{name:"MuiLinearProgress",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`color${P(o.color)}`],n[o.variant]]}})(W(({theme:e})=>({position:"relative",overflow:"hidden",display:"block",height:4,zIndex:0,"@media print":{colorAdjust:"exact"},variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:qe(e,n)}})),{props:({ownerState:n})=>n.color==="inherit"&&n.variant!=="buffer",style:{"&::before":{content:'""',position:"absolute",left:0,top:0,right:0,bottom:0,backgroundColor:"currentColor",opacity:.3}}},{props:{variant:"buffer"},style:{backgroundColor:"transparent"}},{props:{variant:"query"},style:{transform:"rotate(180deg)"}}]}))),Mr=F("span",{name:"MuiLinearProgress",slot:"Dashed",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.dashed,n[`dashedColor${P(o.color)}`]]}})(W(({theme:e})=>({position:"absolute",marginTop:0,height:"100%",width:"100%",backgroundSize:"10px 10px",backgroundPosition:"0 -23px",variants:[{props:{color:"inherit"},style:{opacity:.3,backgroundImage:"radial-gradient(currentColor 0%, currentColor 16%, transparent 42%)"}},...Object.entries(e.palette).filter(K()).map(([n])=>{const o=qe(e,n);return{props:{color:n},style:{backgroundImage:`radial-gradient(${o} 0%, ${o} 16%, transparent 42%)`}}})]})),Pr||{animation:`${Ve} 3s infinite linear`}),Dr=F("span",{name:"MuiLinearProgress",slot:"Bar1",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar1,n[`barColor${P(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar1Indeterminate,o.variant==="determinate"&&n.bar1Determinate,o.variant==="buffer"&&n.bar1Buffer]}})(W(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[{props:{color:"inherit"},style:{backgroundColor:"currentColor"}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:(e.vars||e).palette[n].main}})),{props:{variant:"determinate"},style:{transition:`transform .${Oe}s linear`}},{props:{variant:"buffer"},style:{zIndex:1,transition:`transform .${Oe}s linear`}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:Tr||{animation:`${ze} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite`}}]}))),Or=F("span",{name:"MuiLinearProgress",slot:"Bar2",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar2,n[`barColor${P(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar2Indeterminate,o.variant==="buffer"&&n.bar2Buffer]}})(W(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{"--LinearProgressBar2-barColor":(e.vars||e).palette[n].main}})),{props:({ownerState:n})=>n.variant!=="buffer"&&n.color!=="inherit",style:{backgroundColor:"var(--LinearProgressBar2-barColor, currentColor)"}},{props:({ownerState:n})=>n.variant!=="buffer"&&n.color==="inherit",style:{backgroundColor:"currentColor"}},{props:{color:"inherit"},style:{opacity:.3}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n,variant:"buffer"},style:{backgroundColor:qe(e,n),transition:`transform .${Oe}s linear`}})),{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:Rr||{animation:`${Fe} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite`}}]}))),zr=p.forwardRef(function(n,o){const r=Te({props:n,name:"MuiLinearProgress"}),{className:i,color:u="primary",value:m,valueBuffer:l,variant:g="indeterminate",...C}=r,a={...r,color:u,variant:g},f=Lr(a),E=yr(),x={},s={bar1:{},bar2:{}};if((g==="determinate"||g==="buffer")&&m!==void 0){x["aria-valuenow"]=Math.round(m),x["aria-valuemin"]=0,x["aria-valuemax"]=100;let h=m-100;E&&(h=-h),s.bar1.transform=`translateX(${h}%)`}if(g==="buffer"&&l!==void 0){let h=(l||0)-100;E&&(h=-h),s.bar2.transform=`translateX(${h}%)`}return t.jsxs(Nr,{className:zt(f.root,i),ownerState:a,role:"progressbar",...x,ref:o,...C,children:[g==="buffer"?t.jsx(Mr,{className:f.dashed,ownerState:a}):null,t.jsx(Dr,{className:f.bar1,ownerState:a,style:s.bar1}),g==="determinate"?null:t.jsx(Or,{className:f.bar2,ownerState:a,style:s.bar2})]})});function Fr(e={}){const{autoHideDuration:n=null,disableWindowBlurListener:o=!1,onClose:r,open:i,resumeHideDuration:u}=e,m=dr();p.useEffect(()=>{if(!i)return;function b(y){y.defaultPrevented||y.key==="Escape"&&(r==null||r(y,"escapeKeyDown"))}return document.addEventListener("keydown",b),()=>{document.removeEventListener("keydown",b)}},[i,r]);const l=Ne((b,y)=>{r==null||r(b,y)}),g=Ne(b=>{!r||b==null||m.start(b,()=>{l(null,"timeout")})});p.useEffect(()=>(i&&g(n),m.clear),[i,n,g,m]);const C=b=>{r==null||r(b,"clickaway")},a=m.clear,f=p.useCallback(()=>{n!=null&&g(u??n*.5)},[n,u,g]),E=b=>y=>{const S=b.onBlur;S==null||S(y),f()},x=b=>y=>{const S=b.onFocus;S==null||S(y),a()},s=b=>y=>{const S=b.onMouseEnter;S==null||S(y),a()},h=b=>y=>{const S=b.onMouseLeave;S==null||S(y),f()};return p.useEffect(()=>{if(!o&&i)return window.addEventListener("focus",f),window.addEventListener("blur",a),()=>{window.removeEventListener("focus",f),window.removeEventListener("blur",a)}},[o,i,f,a]),{getRootProps:(b={})=>{const y={...ln(e),...ln(b)};return{role:"presentation",...b,...y,onBlur:E(y),onFocus:x(y),onMouseEnter:s(y),onMouseLeave:h(y)}},onClickAway:C}}function Vr(e){return Be("MuiSnackbarContent",e)}Ge("MuiSnackbarContent",["root","message","action"]);const $r=e=>{const{classes:n}=e;return Ue({root:["root"],action:["action"],message:["message"]},Vr,n)},Hr=F(ur,{name:"MuiSnackbarContent",slot:"Root"})(W(({theme:e})=>{const n=e.palette.mode==="light"?.8:.98;return{...e.typography.body2,color:e.vars?e.vars.palette.SnackbarContent.color:e.palette.getContrastText(dn(e.palette.background.default,n)),backgroundColor:e.vars?e.vars.palette.SnackbarContent.bg:dn(e.palette.background.default,n),display:"flex",alignItems:"center",flexWrap:"wrap",padding:"6px 16px",flexGrow:1,[e.breakpoints.up("sm")]:{flexGrow:"initial",minWidth:288}}})),Br=F("div",{name:"MuiSnackbarContent",slot:"Message"})({padding:"8px 0"}),Gr=F("div",{name:"MuiSnackbarContent",slot:"Action"})({display:"flex",alignItems:"center",marginLeft:"auto",paddingLeft:16,marginRight:-8}),Ur=p.forwardRef(function(n,o){const r=Te({props:n,name:"MuiSnackbarContent"}),{action:i,className:u,message:m,role:l="alert",...g}=r,C=r,a=$r(C);return t.jsxs(Hr,{role:l,elevation:6,className:zt(a.root,u),ownerState:C,ref:o,...g,children:[t.jsx(Br,{className:a.message,ownerState:C,children:m}),i?t.jsx(Gr,{className:a.action,ownerState:C,children:i}):null]})});function _r(e){return Be("MuiSnackbar",e)}Ge("MuiSnackbar",["root","anchorOriginTopCenter","anchorOriginBottomCenter","anchorOriginTopRight","anchorOriginBottomRight","anchorOriginTopLeft","anchorOriginBottomLeft"]);const Wr=e=>{const{classes:n,anchorOrigin:o}=e,r={root:["root",`anchorOrigin${P(o.vertical)}${P(o.horizontal)}`]};return Ue(r,_r,n)},Zr=F("div",{name:"MuiSnackbar",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`anchorOrigin${P(o.anchorOrigin.vertical)}${P(o.anchorOrigin.horizontal)}`]]}})(W(({theme:e})=>({zIndex:(e.vars||e).zIndex.snackbar,position:"fixed",display:"flex",left:8,right:8,justifyContent:"center",alignItems:"center",variants:[{props:({ownerState:n})=>n.anchorOrigin.vertical==="top",style:{top:8,[e.breakpoints.up("sm")]:{top:24}}},{props:({ownerState:n})=>n.anchorOrigin.vertical!=="top",style:{bottom:8,[e.breakpoints.up("sm")]:{bottom:24}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="left",style:{justifyContent:"flex-start",[e.breakpoints.up("sm")]:{left:24,right:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="right",style:{justifyContent:"flex-end",[e.breakpoints.up("sm")]:{right:24,left:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="center",style:{[e.breakpoints.up("sm")]:{left:"50%",right:"auto",transform:"translateX(-50%)"}}}]}))),qr=p.forwardRef(function(n,o){const r=Te({props:n,name:"MuiSnackbar"}),i=pr(),u={enter:i.transitions.duration.enteringScreen,exit:i.transitions.duration.leavingScreen},{action:m,anchorOrigin:{vertical:l,horizontal:g}={vertical:"bottom",horizontal:"left"},autoHideDuration:C=null,children:a,className:f,ClickAwayListenerProps:E,ContentProps:x,disableWindowBlurListener:s=!1,message:h,onBlur:k,onClose:b,onFocus:y,onMouseEnter:S,onMouseLeave:Z,open:V,resumeHideDuration:J,slots:d={},slotProps:c={},TransitionComponent:w,transitionDuration:M=u,TransitionProps:{onEnter:q,onExited:$,...Zt}={},...qt}=r,H={...r,anchorOrigin:{vertical:l,horizontal:g},autoHideDuration:C,disableWindowBlurListener:s,TransitionComponent:w,transitionDuration:M},Yt=Wr(H),{getRootProps:Kt,onClickAway:Xt}=Fr({...H}),[Jt,Ke]=p.useState(!0),Qt=R=>{Ke(!0),$&&$(R)},eo=(R,N)=>{Ke(!1),q&&q(R,N)},Q={slots:{transition:w,...d},slotProps:{content:x,clickAwayListener:E,transition:Zt,...c}},[no,to]=ne("root",{ref:o,className:[Yt.root,f],elementType:Zr,getSlotProps:Kt,externalForwardedProps:{...Q,...qt},ownerState:H}),[oo,{ownerState:ro,...so}]=ne("clickAwayListener",{elementType:Sr,externalForwardedProps:Q,getSlotProps:R=>({onClickAway:(...N)=>{var Xe;const O=N[0];(Xe=R.onClickAway)==null||Xe.call(R,...N),!(O!=null&&O.defaultMuiPrevented)&&Xt(...N)}}),ownerState:H}),[ao,io]=ne("content",{elementType:Ur,shouldForwardComponentProp:!0,externalForwardedProps:Q,additionalProps:{message:h,action:m},ownerState:H}),[co,lo]=ne("transition",{elementType:Yo,externalForwardedProps:Q,getSlotProps:R=>({onEnter:(...N)=>{var O;(O=R.onEnter)==null||O.call(R,...N),eo(...N)},onExited:(...N)=>{var O;(O=R.onExited)==null||O.call(R,...N),Qt(...N)}}),additionalProps:{appear:!0,in:V,timeout:M,direction:l==="top"?"down":"up"},ownerState:H});return!V&&Jt?null:t.jsx(oo,{...so,...d.clickAwayListener&&{ownerState:ro},children:t.jsx(no,{...to,children:t.jsx(co,{...lo,children:a||t.jsx(ao,{...io})})})})}),Yr=He(t.jsx("path",{d:"M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2zm10 14.5V20H8v-3.5l4-4zm-4-5-4-4V4h8v3.5z"})),Kr=He(t.jsx("path",{d:"M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2m6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1zm-2 1H8v-6c0-2.48 1.51-4.5 4-4.5s4 2.02 4 4.5z"})),Xr=He([t.jsx("path",{d:"M12 5.99 19.53 19H4.47zM12 2 1 21h22z"},"0"),t.jsx("path",{d:"M13 16h-2v2h2zm0-6h-2v5h2z"},"1")]),Jr=e=>e.level===Le.ERROR?t.jsx(Mt,{color:"error",fontSize:"small"}):e.level===Le.WARNING?t.jsx(Xr,{color:"warning",fontSize:"small"}):t.jsx(rr,{color:"info",fontSize:"small"}),Qr=e=>{try{const n=new URL(e,window.location.origin);return n.origin!==window.location.origin?null:`${n.pathname}${n.search}${n.hash}`}catch{return null}},Gt=()=>{const e=$e(),n=Dt(),o=v(uo),[r,i]=p.useState(null),[u,m]=p.useState(null),l=!!r,g=!!u,C=v(po),a=v(mo),f=v(go),E=v(fo),x=v(ho),s=p.useMemo(()=>a.filter(d=>!d.viewed).map(d=>d.id),[a]);p.useEffect(()=>{o&&f==="idle"&&e(bo())},[o,e,f]);const h=d=>{i(d.currentTarget)},k=()=>{i(null)},b=()=>{k(),e(xo())},y=d=>{m(d.currentTarget),s.length>0&&e(Co(s))},S=()=>{m(null)},Z=d=>{if(!d.actionUrl||!d.canAction)return;S();const c=Qr(d.actionUrl);if(c){n(c);return}window.location.href=d.actionUrl},V=C.length==0?"":C.map((d,c)=>t.jsx(qr,{open:!0,autoHideDuration:6e3,onClose:()=>e(Je(c)),anchorOrigin:{vertical:"bottom",horizontal:"center"},children:t.jsx(Y,{onClose:()=>e(Je(c)),severity:d.type,sx:{width:"100%"},children:d.message})},d.id)),J=a.length===0?t.jsx(ee,{disabled:!0,children:t.jsx(an,{primary:"No notifications",primaryTypographyProps:{variant:"body2",color:"text.secondary"}})}):a.map(d=>t.jsxs(ee,{disableRipple:!0,sx:{alignItems:"flex-start",gap:1.5,maxWidth:420,minWidth:{xs:300,sm:380},py:1.5,whiteSpace:"normal"},children:[t.jsx(or,{sx:{minWidth:32,pt:.25},children:Jr(d)}),t.jsx(an,{primary:d.title,secondary:d.message,primaryTypographyProps:{variant:"subtitle2",fontWeight:d.viewed?500:700},secondaryTypographyProps:{variant:"body2",color:"text.secondary",sx:{mt:.5}}}),d.canAction&&d.actionUrl&&t.jsx(_,{size:"small",variant:"outlined",onClick:c=>{c.stopPropagation(),Z(d)},sx:{flexShrink:0,mt:.25},children:"Open"})]},d.id));return t.jsxs(T,{sx:{display:"flex",flexDirection:"column",minHeight:"100vh",bgcolor:"background.default"},children:[o&&t.jsxs(Ze,{maxWidth:"lg",sx:{display:"flex",justifyContent:"flex-end",alignItems:"center",gap:1,pt:{xs:1,sm:2}},children:[t.jsx(Jo,{title:"Open notifications",children:t.jsx(gr,{id:"notifications-button",color:"inherit",size:"small",onClick:y,"aria-controls":g?"notifications-menu":void 0,"aria-haspopup":"true","aria-expanded":g?"true":void 0,"aria-label":"Open notifications",sx:{color:"text.secondary"},children:t.jsx(Qo,{badgeContent:x,color:"warning",invisible:x===0,children:t.jsx(Kr,{fontSize:"small"})})})}),t.jsxs(en,{id:"notifications-menu",anchorEl:u,open:g,onClose:S,MenuListProps:{"aria-labelledby":"notifications-button"},PaperProps:{sx:{mt:1,maxHeight:480,borderRadius:j.radius.panel}},children:[t.jsxs(T,{sx:{px:2,py:1.25},children:[t.jsx(A,{variant:"subtitle1",component:"p",sx:{fontWeight:700},children:"Notifications"}),f==="failed"&&t.jsx(A,{variant:"body2",color:"error",children:E??"Could not load notifications"})]}),t.jsx(er,{}),J]}),t.jsx(_,{id:"account-button",onClick:h,color:"inherit",size:"small",endIcon:t.jsx(nr,{alt:o,src:"/assets/avatar.png",sx:{width:28,height:28,fontSize:14}}),"aria-controls":l?"account-menu":void 0,"aria-haspopup":"true","aria-expanded":l?"true":void 0,sx:{color:"text.secondary",minWidth:0,textTransform:"none"},children:t.jsx(A,{variant:"body2",component:"span",noWrap:!0,sx:{display:{xs:"none",sm:"inline"},maxWidth:260},children:o})}),t.jsxs(en,{id:"account-menu",anchorEl:r,open:l,onClose:k,MenuListProps:{"aria-labelledby":"account-button"},children:[t.jsx(ee,{disabled:!0,children:t.jsx(A,{variant:"body2",children:o})}),t.jsx(ee,{onClick:b,children:"Logout"})]})]}),t.jsxs(T,{component:"main",sx:{flexGrow:1},children:[t.jsx(ir,{}),V]})]})};Gt.__docgenInfo={description:"Layout component for the application",methods:[],displayName:"Layout"};const Ut=()=>{const e=$e(),n=Dt(),o=v(Rt),r=v(Pt),i=v(Lt),{cancelForm:u,connect:m,currentFormStep:l,formSubmitError:g,isConnecting:C,isSubmittingForm:a,submitForm:f}=fr();p.useEffect(()=>{r==="idle"&&e(Nt())},[r,e]);const E=p.useCallback(s=>{n(`/connectors/${encodeURIComponent(s)}`)},[n]);let x;return r==="loading"?x=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Ht,{})},s))}):r==="failed"?x=t.jsx(Y,{severity:"error",children:i}):o.length===0?x=t.jsx(T,{sx:{textAlign:"center",py:j.spacing.pageY},children:t.jsx(A,{variant:"h6",color:"text.secondary",children:"No connectors available"})}):x=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:o.map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx($t,{connector:s,onConnect:m,onDetails:E,isConnecting:C})},s.id))}),t.jsxs(Ze,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(A,{variant:"h4",component:"h1",children:"Available Connectors"}),t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:j.spacing.headerGap},children:[C&&t.jsxs(T,{sx:{display:"flex",alignItems:"center"},children:[t.jsx(Me,{size:24,sx:{mr:1}}),t.jsx(A,{variant:"body2",color:"text.secondary",children:"Connecting..."})]}),t.jsx(_,{component:Ft,to:"/connections",startIcon:t.jsx(hr,{}),sx:{alignSelf:{xs:"flex-start",sm:"center"}},children:"Back to Connections"})]})]}),x,t.jsx(Vt,{currentFormStep:l,formSubmitError:g,isSubmittingForm:a,onCancel:u,onSubmit:f})]})};Ut.__docgenInfo={description:"Component to display a list of available connectors",methods:[],displayName:"ConnectorList"};const _t=()=>{const e=$e(),[n,o]=br(),r=v(yo),i=v(vo),u=v(So),m=v(Rt),l=v(Pt),g=v(Lt),C=v(jo),a=v(wo),f=v(ko),E=v(Ao),x=v(Io),s=v(Eo),h=v(To),k=v(Ro);p.useEffect(()=>{i==="idle"&&e(B()),l==="idle"&&e(Nt())},[i,l,e]),p.useEffect(()=>{const c=n.get("setup"),w=n.get("connectionId");c==="pending"&&w&&(e(Qe(w)),n.delete("setup"),n.delete("connectionId"),o(n,{replace:!0}))},[n,o,e]),p.useEffect(()=>{if(!x)return;const c=window.setInterval(()=>{e(Qe(x))},2e3);return()=>window.clearInterval(c)},[x,e]),p.useEffect(()=>{if(!k)return;e(B());const c=window.setTimeout(()=>{e(Po())},3500);return()=>window.clearTimeout(c)},[e,k]);const b=p.useCallback((c,w)=>{const M=(a==null?void 0:a.stepId)??"";e(Lo({connectionId:c,stepId:M,data:w,returnToUrl:window.location.href})).then(q=>{if(q.meta.requestStatus==="fulfilled"){const $=q.payload;nn($)?window.location.href=$.redirectUrl:tn($)?e(B()):e(B())}})},[e,a]),y=p.useCallback(()=>{const c=a==null?void 0:a.connectionId,w=c?r.find(M=>M.id===c):void 0;w&&w.state===Ie.CONFIGURED&&e(No(w.id)),e(Mo())},[e,a,r]),S=p.useCallback(()=>{s&&e(Do({connectionId:s.connectionId,returnToUrl:window.location.href})).then(c=>{if(c.meta.requestStatus==="fulfilled"){const w=c.payload;w.type==="redirect"&&w.redirectUrl&&(window.location.href=w.redirectUrl)}})},[e,s]),Z=p.useCallback(()=>{s&&e(Oo(s.connectionId)).then(()=>{e(zo()),e(B())})},[e,s]),V=p.useCallback(c=>{e(Fo({connectorId:c,returnToUrl:`${window.location.origin}/connections`})).then(w=>{if(w.meta.requestStatus==="fulfilled"){const M=w.payload;nn(M)?window.location.href=M.redirectUrl:tn(M)&&e(B())}})},[e]),J=()=>l==="loading"||l==="idle"?t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Ht,{})},`connector-skeleton-${c}`))}):l==="failed"?t.jsx(Y,{severity:"error",children:g}):m.length===0?t.jsx(T,{sx:{py:3},children:t.jsx(A,{color:"text.secondary",children:"No connectors are available right now."})}):t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:m.map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx($t,{connector:c,onConnect:V,isConnecting:C})},c.id))});let d;return i==="loading"?d=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(ar,{})},`connection-skeleton-${c}`))}):i==="failed"?d=t.jsx(Y,{severity:"error",children:u}):r.length===0?d=t.jsxs(t.Fragment,{children:[t.jsxs(T,{sx:{border:1,borderColor:j.card.borderColor,borderRadius:j.radius.panel,bgcolor:j.card.surface,mb:j.spacing.sectionGap,p:j.spacing.panelPadding},children:[t.jsx(A,{variant:"h5",component:"h2",gutterBottom:!0,children:"Connect your first application"}),t.jsx(A,{color:"text.secondary",sx:{maxWidth:680},children:"Choose a connector below to create a connection. Once connected, it will appear here for ongoing setup, health, and management."}),C&&t.jsxs(T,{sx:{display:"flex",alignItems:"center",mt:3},children:[t.jsx(Me,{size:24,sx:{mr:1}}),t.jsx(A,{variant:"body2",color:"text.secondary",children:"Starting connection..."})]})]}),t.jsxs(T,{children:[t.jsx(A,{variant:"h6",component:"h2",sx:{mb:2},children:"Available connectors"}),J()]})]}):d=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:r.map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(sr,{connection:c,highlightNew:c.id===k})},c.id))}),t.jsxs(Ze,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(A,{variant:"h4",component:"h1",children:"Your Connections"}),r.length>0&&t.jsx(_,{variant:"contained",color:"primary",startIcon:t.jsx(tr,{}),component:Ft,to:"/connectors",children:"Connect More"})]}),d,t.jsx(Vt,{currentFormStep:a,formSubmitError:E,isSubmittingForm:f,onCancel:y,onSubmit:b}),t.jsxs(on,{open:x!==null,maxWidth:"xs",fullWidth:!0,children:[t.jsx(rn,{sx:{pb:1},children:"Verifying connection"}),t.jsx(sn,{dividers:!0,children:t.jsxs(T,{sx:{display:"flex",flexDirection:"column",alignItems:"center",gap:j.spacing.headerGap,py:3},children:[t.jsx(Yr,{color:"primary",sx:{fontSize:40}}),t.jsxs(T,{sx:{textAlign:"center"},children:[t.jsx(A,{variant:"subtitle1",component:"p",children:"Checking credentials"}),t.jsx(A,{variant:"body2",color:"text.secondary",children:"AuthProxy is confirming that this connection can reach the provider."})]}),t.jsx(zr,{sx:{width:"100%"}})]})})]}),t.jsxs(on,{open:s!==null,onClose:Z,maxWidth:"sm",fullWidth:!0,children:[t.jsx(rn,{sx:{pb:1},children:t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:1},children:[t.jsx(Mt,{color:"error"}),t.jsx(A,{variant:"h6",component:"span",children:"Connection verification failed"})]})}),t.jsxs(sn,{dividers:!0,children:[t.jsxs(Y,{severity:"error",sx:{mb:2},children:[t.jsx(Cr,{children:"Provider check failed"}),(s==null?void 0:s.message)??"Verification failed"]}),t.jsx(A,{variant:"body2",color:"text.secondary",children:s!=null&&s.canRetry?"Retry setup to run verification again. Cancel setup deletes this unfinished connection.":"Cancel setup to delete this unfinished connection, then start again from the connector."})]}),t.jsxs(Ko,{children:[t.jsx(_,{onClick:Z,disabled:h,children:"Cancel setup"}),(s==null?void 0:s.canRetry)&&t.jsx(_,{onClick:S,disabled:h,variant:"contained",startIcon:h?t.jsx(Me,{size:16}):void 0,children:h?"Retrying setup...":"Retry setup"})]})]})]})};_t.__docgenInfo={description:"Component to display a list of connections",methods:[],displayName:"ConnectionList"};const U=(e,n,o="#ffffff")=>{const r=e.split(/\s+/).map(u=>u[0]).join("").slice(0,2).toUpperCase(),i=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${n}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${o}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${r}</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(i)}`},D=[{id:"google-drive",namespace:"root",version:1,state:G.ACTIVE,displayName:"Google Drive",description:"Have the agent track your work in Google Drive.",highlight:"Have the agent track your work in Google Drive.",logo:U("Google Drive","#188038"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"greenhouse",namespace:"root",version:1,state:G.ACTIVE,displayName:"Greenhouse",description:"This integration pushes candidates to greenhouse.",highlight:"This integration pushes candidates to greenhouse.",logo:U("Greenhouse","#24a47f"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"google-calendar",namespace:"root",version:1,state:G.ACTIVE,displayName:"Google Calendar",description:`Google Calendar lets agents coordinate scheduling work without needing direct access to your primary app.

![Calendar workflow preview](/calendar-workflow-preview.svg)

### What agents can do

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create and update events | Yes |
| Read attendee responses | Yes |
| Manage private event details | No |

Use this connector when the assistant should propose meeting times, create holds, or keep follow-up work attached to calendar events.`,highlight:"Coordinate meetings, availability, and follow-up from Google Calendar.",logo:U("Google Calendar","#1a73e8"),hasConfigure:!0,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"gmail",namespace:"root",version:1,state:G.ACTIVE,displayName:"GMail",description:"Have the agent respond to your emails without you needing to be involved. Like magic.",highlight:"Have the agent respond to your emails without you needing to be involved. Like magic.",logo:U("GMail","#d93025"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"pipedrive",namespace:"root",version:1,state:G.ACTIVE,displayName:"pipedrive",description:"Allow our agent to handle your sales support.",highlight:"Allow our agent to handle your sales support.",logo:U("pipedrive","#017a5e"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"},{id:"asana",namespace:"root",version:1,state:G.ACTIVE,displayName:"Asana",description:"Allow our agent organize your work.",highlight:"Allow our agent organize your work.",logo:U("Asana","#f06a6a"),hasConfigure:!1,createdAt:"2024-01-01T00:00:00Z",updatedAt:"2024-01-01T00:00:00Z"}],z=(e,n={})=>({id:`cxn_${e.id}`,namespace:"root",connector:e,state:Ie.CONFIGURED,healthState:Ee.HEALTHY,createdAt:"2024-04-01T12:00:00Z",updatedAt:"2024-04-01T12:00:00Z",...n}),es=[z(D[0]),z(D[2],{healthState:Ee.UNHEALTHY}),z(D[5],{state:Ie.SETUP}),z(D[4],{state:Ie.DISABLED})],Ye={connectionId:"cxn_google-calendar",stepId:"select-calendar",stepTitle:"Select a Calendar",stepDescription:"Choose which Google Calendar the agent should manage.",currentStep:0,totalSteps:2,jsonSchema:{type:"object",required:["calendar_id"],properties:{calendar_id:{type:"string",title:"Calendar",enum:["primary","product","support"]}}},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar_id"}]}},I={items:es,status:"succeeded",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null,currentFormStep:null,submittingForm:!1,formSubmitError:null,verifyingConnectionId:null,verifyError:null,retryingConnection:!1,recentlyCompletedConnectionId:null},Wt={items:[],status:"succeeded",error:null,markingViewed:!1};function ns({route:e,connectorsState:n={items:D,status:"succeeded",error:null},connectionsState:o=I,notificationsState:r=Wt}){const i=$o({reducer:Ho({auth:Zo,connectors:Wo,connections:_o,notifications:Uo,toasts:Go}),preloadedState:{auth:{actorId:"actor_storybook",status:"authenticated"},connectors:n,connections:o,notifications:r,toasts:{items:[]}}});return t.jsx(Bo,{store:i,children:t.jsxs(mr,{theme:Xo,children:[t.jsx(Ir,{}),t.jsx(cr,{children:t.jsx(cn,{element:t.jsx(Gt,{}),children:t.jsx(cn,{path:"*",element:e==="/connectors"?t.jsx(Ut,{}):e==="/connector-detail"?t.jsx(xr,{connectorId:"google-calendar"}):t.jsx(_t,{})})})})]})})}const Is={title:"Pages/Marketplace",component:ns,parameters:{layout:"fullscreen"}},Re={viewport:{defaultViewport:"marketplaceMobile"}},X={viewport:{defaultViewport:"marketplaceTablet"}},te={args:{route:"/connectors"}},oe={args:{route:"/connector-detail"}},re={args:{route:"/connector-detail"},parameters:Re},se={args:{route:"/connectors",connectorsState:{items:[],status:"loading",error:null}}},ae={args:{route:"/connections"}},ie={args:{route:"/connections"},parameters:Re},ce={args:{route:"/connections"},parameters:X},le={args:{route:"/connections",connectionsState:{...I,items:[z(D[2],{healthState:Ee.UNHEALTHY})]}}},de={args:{route:"/connections",connectionsState:{...I,items:[z(D[2],{healthState:Ee.UNHEALTHY}),z(D[5],{setupStepId:"select-workspace"})]},notificationsState:{...Wt,items:[{id:"ntf_reauth",key:"connection:cxn_google-calendar:auth_required",level:Le.WARNING,state:Vo.ACTIVE,resourceType:"connection",resourceId:"cxn_google-calendar",namespace:"root",title:"Connection requires re-authentication",message:"Reconnect this connection to continue using it.",actionUrl:"/connections/cxn_google-calendar?action=reauth",canAction:!0,viewed:!1,createdAt:"2026-07-12T12:00:00Z",updatedAt:"2026-07-12T12:00:00Z"}]}}},ue={args:{route:"/connections",connectionsState:{...I,items:[z(D[2]),z(D[0])]}}},pe={args:{route:"/connections",connectionsState:{...I,items:[]}}},me={args:{route:"/connections",connectionsState:{...I,items:[]}},parameters:Re},ge={args:{route:"/connections",connectionsState:{...I,items:[]}},parameters:X},fe={args:{route:"/connections",connectorsState:{items:[],status:"loading",error:null},connectionsState:{...I,items:[]}}},he={args:{route:"/connectors"},parameters:Re},be={args:{route:"/connectors"},parameters:X},Ce={args:{route:"/connections",connectionsState:{...I,currentFormStep:Ye}}},xe={args:{route:"/connections",connectionsState:{...I,currentFormStep:Ye}},parameters:X},ye={args:{route:"/connections",connectionsState:{...I,currentFormStep:Ye,submittingForm:!0}}},ve={args:{route:"/connections",connectionsState:{...I,verifyingConnectionId:"cxn_google-calendar"}}},Se={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}}},je={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}},parameters:X},we={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0},retryingConnection:!0}}},ke={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"The provider rejected this setup and it cannot be retried.",canRetry:!1}}}};var pn,mn,gn;te.parameters={...te.parameters,docs:{...(pn=te.parameters)==null?void 0:pn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  }
}`,...(gn=(mn=te.parameters)==null?void 0:mn.docs)==null?void 0:gn.source}}};var fn,hn,bn;oe.parameters={...oe.parameters,docs:{...(fn=oe.parameters)==null?void 0:fn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  }
}`,...(bn=(hn=oe.parameters)==null?void 0:hn.docs)==null?void 0:bn.source}}};var Cn,xn,yn;re.parameters={...re.parameters,docs:{...(Cn=re.parameters)==null?void 0:Cn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  },
  parameters: mobileViewport
}`,...(yn=(xn=re.parameters)==null?void 0:xn.docs)==null?void 0:yn.source}}};var vn,Sn,jn;se.parameters={...se.parameters,docs:{...(vn=se.parameters)==null?void 0:vn.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    }
  }
}`,...(jn=(Sn=se.parameters)==null?void 0:Sn.docs)==null?void 0:jn.source}}};var wn,kn,An;ae.parameters={...ae.parameters,docs:{...(wn=ae.parameters)==null?void 0:wn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  }
}`,...(An=(kn=ae.parameters)==null?void 0:kn.docs)==null?void 0:An.source}}};var In,En,Tn;ie.parameters={...ie.parameters,docs:{...(In=ie.parameters)==null?void 0:In.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: mobileViewport
}`,...(Tn=(En=ie.parameters)==null?void 0:En.docs)==null?void 0:Tn.source}}};var Rn,Pn,Ln;ce.parameters={...ce.parameters,docs:{...(Rn=ce.parameters)==null?void 0:Rn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: tabletViewport
}`,...(Ln=(Pn=ce.parameters)==null?void 0:Pn.docs)==null?void 0:Ln.source}}};var Nn,Mn,Dn;le.parameters={...le.parameters,docs:{...(Nn=le.parameters)==null?void 0:Nn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        healthState: ConnectionHealthState.UNHEALTHY
      })]
    }
  }
}`,...(Dn=(Mn=le.parameters)==null?void 0:Mn.docs)==null?void 0:Dn.source}}};var On,zn,Fn;de.parameters={...de.parameters,docs:{...(On=de.parameters)==null?void 0:On.docs,source:{originalSource:`{
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
}`,...(Fn=(zn=de.parameters)==null?void 0:zn.docs)==null?void 0:Fn.source}}};var Vn,$n,Hn;ue.parameters={...ue.parameters,docs:{...(Vn=ue.parameters)==null?void 0:Vn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2]), connectionFor(connectors[0])]
    }
  }
}`,...(Hn=($n=ue.parameters)==null?void 0:$n.docs)==null?void 0:Hn.source}}};var Bn,Gn,Un;pe.parameters={...pe.parameters,docs:{...(Bn=pe.parameters)==null?void 0:Bn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Un=(Gn=pe.parameters)==null?void 0:Gn.docs)==null?void 0:Un.source}}};var _n,Wn,Zn;me.parameters={...me.parameters,docs:{...(_n=me.parameters)==null?void 0:_n.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: mobileViewport
}`,...(Zn=(Wn=me.parameters)==null?void 0:Wn.docs)==null?void 0:Zn.source}}};var qn,Yn,Kn;ge.parameters={...ge.parameters,docs:{...(qn=ge.parameters)==null?void 0:qn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: tabletViewport
}`,...(Kn=(Yn=ge.parameters)==null?void 0:Yn.docs)==null?void 0:Kn.source}}};var Xn,Jn,Qn;fe.parameters={...fe.parameters,docs:{...(Xn=fe.parameters)==null?void 0:Xn.docs,source:{originalSource:`{
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
}`,...(Qn=(Jn=fe.parameters)==null?void 0:Jn.docs)==null?void 0:Qn.source}}};var et,nt,tt;he.parameters={...he.parameters,docs:{...(et=he.parameters)==null?void 0:et.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: mobileViewport
}`,...(tt=(nt=he.parameters)==null?void 0:nt.docs)==null?void 0:tt.source}}};var ot,rt,st;be.parameters={...be.parameters,docs:{...(ot=be.parameters)==null?void 0:ot.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: tabletViewport
}`,...(st=(rt=be.parameters)==null?void 0:rt.docs)==null?void 0:st.source}}};var at,it,ct;Ce.parameters={...Ce.parameters,docs:{...(at=Ce.parameters)==null?void 0:at.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  }
}`,...(ct=(it=Ce.parameters)==null?void 0:it.docs)==null?void 0:ct.source}}};var lt,dt,ut;xe.parameters={...xe.parameters,docs:{...(lt=xe.parameters)==null?void 0:lt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  },
  parameters: tabletViewport
}`,...(ut=(dt=xe.parameters)==null?void 0:dt.docs)==null?void 0:ut.source}}};var pt,mt,gt;ye.parameters={...ye.parameters,docs:{...(pt=ye.parameters)==null?void 0:pt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true
    }
  }
}`,...(gt=(mt=ye.parameters)==null?void 0:mt.docs)==null?void 0:gt.source}}};var ft,ht,bt;ve.parameters={...ve.parameters,docs:{...(ft=ve.parameters)==null?void 0:ft.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar'
    }
  }
}`,...(bt=(ht=ve.parameters)==null?void 0:ht.docs)==null?void 0:bt.source}}};var Ct,xt,yt;Se.parameters={...Se.parameters,docs:{...(Ct=Se.parameters)==null?void 0:Ct.docs,source:{originalSource:`{
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
}`,...(yt=(xt=Se.parameters)==null?void 0:xt.docs)==null?void 0:yt.source}}};var vt,St,jt;je.parameters={...je.parameters,docs:{...(vt=je.parameters)==null?void 0:vt.docs,source:{originalSource:`{
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
}`,...(jt=(St=je.parameters)==null?void 0:St.docs)==null?void 0:jt.source}}};var wt,kt,At;we.parameters={...we.parameters,docs:{...(wt=we.parameters)==null?void 0:wt.docs,source:{originalSource:`{
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
}`,...(At=(kt=we.parameters)==null?void 0:kt.docs)==null?void 0:At.source}}};var It,Et,Tt;ke.parameters={...ke.parameters,docs:{...(It=ke.parameters)==null?void 0:It.docs,source:{originalSource:`{
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
}`,...(Tt=(Et=ke.parameters)==null?void 0:Et.docs)==null?void 0:Tt.source}}};const Es=["AvailableConnectors","ConnectorOverview","ConnectorOverviewMobile","AvailableConnectorsLoading","ConnectionsPopulated","ConnectionsPopulatedMobile","ConnectionsPopulatedTablet","ConnectionsNeedsAttention","ConnectionsWithNotifications","ConnectionsHealthyActions","ConnectionsEmpty","ConnectionsEmptyMobile","ConnectionsEmptyTablet","ConnectionsEmptyLoadingConnectors","AvailableConnectorsMobile","AvailableConnectorsTablet","ConnectionSetupDialog","ConnectionSetupDialogTablet","ConnectionSetupSubmitting","VerifyingConnectionDialog","VerificationFailedDialog","VerificationFailedDialogTablet","VerificationRetryingDialog","VerificationFailedNoRetryDialog"];export{te as AvailableConnectors,se as AvailableConnectorsLoading,he as AvailableConnectorsMobile,be as AvailableConnectorsTablet,Ce as ConnectionSetupDialog,xe as ConnectionSetupDialogTablet,ye as ConnectionSetupSubmitting,pe as ConnectionsEmpty,fe as ConnectionsEmptyLoadingConnectors,me as ConnectionsEmptyMobile,ge as ConnectionsEmptyTablet,ue as ConnectionsHealthyActions,le as ConnectionsNeedsAttention,ae as ConnectionsPopulated,ie as ConnectionsPopulatedMobile,ce as ConnectionsPopulatedTablet,de as ConnectionsWithNotifications,oe as ConnectorOverview,re as ConnectorOverviewMobile,Se as VerificationFailedDialog,je as VerificationFailedDialogTablet,ke as VerificationFailedNoRetryDialog,we as VerificationRetryingDialog,ve as VerifyingConnectionDialog,Es as __namedExportsOrder,Is as default};
