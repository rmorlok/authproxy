import{r as N}from"./index-yIsmwZOr.js";import{i as R,j as w,k as I,l as E,c as D,g as F,a as B,u as U,s as x,f as h,b as z,m as P,h as A,n as T,o as j,C as G,T as K,p as O}from"./createSimplePaletteValueFilter-VkOLJdQH.js";import{j as d}from"./jsx-runtime-BjG_zV1W.js";function V(e={}){const{themeId:r,defaultTheme:s,defaultClassName:o="MuiBox-root",generateClassName:n}=e,c=R("div",{shouldForwardProp:i=>i!=="theme"&&i!=="sx"&&i!=="as"})(w);return N.forwardRef(function(l,g){const a=I(s),{className:m,component:p="div",...k}=E(l);return d.jsx(c,{as:p,ref:g,className:D(m,n?n(o):o),theme:r&&a[r]||a,...k})})}function W(e){return F("MuiCircularProgress",e)}B("MuiCircularProgress",["root","determinate","indeterminate","colorPrimary","colorSecondary","svg","track","circle","circleDeterminate","circleIndeterminate","circleDisableShrink"]);const t=44,C=j`
  0% {
    transform: rotate(0deg);
  }

  100% {
    transform: rotate(360deg);
  }
`,v=j`
  0% {
    stroke-dasharray: 1px, 200px;
    stroke-dashoffset: 0;
  }

  50% {
    stroke-dasharray: 100px, 200px;
    stroke-dashoffset: -15px;
  }

  100% {
    stroke-dasharray: 1px, 200px;
    stroke-dashoffset: -126px;
  }
`,H=typeof C!="string"?T`
        animation: ${C} 1.4s linear infinite;
      `:null,Z=typeof v!="string"?T`
        animation: ${v} 1.4s ease-in-out infinite;
      `:null,_=e=>{const{classes:r,variant:s,color:o,disableShrink:n}=e,c={root:["root",s,`color${h(o)}`],svg:["svg"],track:["track"],circle:["circle",`circle${h(s)}`,n&&"circleDisableShrink"]};return z(c,W,r)},q=x("span",{name:"MuiCircularProgress",slot:"Root",overridesResolver:(e,r)=>{const{ownerState:s}=e;return[r.root,r[s.variant],r[`color${h(s.color)}`]]}})(P(({theme:e})=>({display:"inline-block",variants:[{props:{variant:"determinate"},style:{transition:e.transitions.create("transform")}},{props:{variant:"indeterminate"},style:H||{animation:`${C} 1.4s linear infinite`}},...Object.entries(e.palette).filter(A()).map(([r])=>({props:{color:r},style:{color:(e.vars||e).palette[r].main}}))]}))),J=x("svg",{name:"MuiCircularProgress",slot:"Svg"})({display:"block"}),L=x("circle",{name:"MuiCircularProgress",slot:"Circle",overridesResolver:(e,r)=>{const{ownerState:s}=e;return[r.circle,r[`circle${h(s.variant)}`],s.disableShrink&&r.circleDisableShrink]}})(P(({theme:e})=>({stroke:"currentColor",variants:[{props:{variant:"determinate"},style:{transition:e.transitions.create("stroke-dashoffset")}},{props:{variant:"indeterminate"},style:{strokeDasharray:"80px, 200px",strokeDashoffset:0}},{props:({ownerState:r})=>r.variant==="indeterminate"&&!r.disableShrink,style:Z||{animation:`${v} 1.4s ease-in-out infinite`}}]}))),Q=x("circle",{name:"MuiCircularProgress",slot:"Track"})(P(({theme:e})=>({stroke:"currentColor",opacity:(e.vars||e).palette.action.activatedOpacity}))),te=N.forwardRef(function(r,s){const o=U({props:r,name:"MuiCircularProgress"}),{className:n,color:c="primary",disableShrink:S=!1,enableTrackSlot:i=!1,size:l=40,style:g,thickness:a=3.6,value:m=0,variant:p="indeterminate",...k}=o,u={...o,color:c,disableShrink:S,size:l,thickness:a,value:m,variant:p,enableTrackSlot:i},f=_(u),y={},b={},$={};if(p==="determinate"){const M=2*Math.PI*((t-a)/2);y.strokeDasharray=M.toFixed(3),$["aria-valuenow"]=Math.round(m),y.strokeDashoffset=`${((100-m)/100*M).toFixed(3)}px`,b.transform="rotate(-90deg)"}return d.jsx(q,{className:D(f.root,n),style:{width:l,height:l,...b,...g},ownerState:u,ref:s,role:"progressbar",...$,...k,children:d.jsxs(J,{className:f.svg,ownerState:u,viewBox:`${t/2} ${t/2} ${t} ${t}`,children:[i?d.jsx(Q,{className:f.track,ownerState:u,cx:t,cy:t,r:(t-a)/2,fill:"none",strokeWidth:a,"aria-hidden":"true"}):null,d.jsx(L,{className:f.circle,style:y,ownerState:u,cx:t,cy:t,r:(t-a)/2,fill:"none",strokeWidth:a})]})})}),X=B("MuiBox",["root"]),Y=O(),ae=V({themeId:K,defaultTheme:Y,defaultClassName:X.root,generateClassName:G.generate});export{ae as B,te as C};
