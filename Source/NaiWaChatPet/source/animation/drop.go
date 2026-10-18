package animation

import (
 "errors"
 "math"
 "time"
)

// DropConfig uses sprite-center x and bottom y in the stable pet canvas.
// These coordinates must be calibrated to actual repaired body pixels.
type DropConfig struct {
 X,StartY,GroundY float64
 InitialVelocity,Gravity,Restitution float64
 RevealTime,SquashTime time.Duration
 SettleSpeed float64
}

type DropPose struct {
 X,BottomY,ScaleX,ScaleY,Reveal,ShadowOpacity,ShadowScale float64
 Settled bool
}

func (c DropConfig) Validate() error {
 for _,v:=range []float64{c.X,c.StartY,c.GroundY,c.InitialVelocity,c.Gravity,c.Restitution,c.SettleSpeed} {
  if math.IsNaN(v)||math.IsInf(v,0) { return errors.New("nonfinite drop parameter") }
 }
 if c.StartY>=c.GroundY || c.InitialVelocity<0 || c.Gravity<=0 || c.Restitution<=0 || c.Restitution>=1 || c.SettleSpeed<=0 || c.RevealTime<=0 || c.SquashTime<=0 { return errors.New("invalid drop trajectory") }
 return nil
}

// DropAt analytically solves collisions; large timer gaps cannot tunnel through
// the floor. It is a function of event age, never prior draw state.
func DropAt(c DropConfig,age time.Duration) (DropPose,error) {
 if err:=c.Validate();err!=nil { return DropPose{},err }
 if age<0 { return DropPose{},errors.New("negative drop age") }
 t:=age.Seconds()
 hit:=(-c.InitialVelocity+math.Sqrt(c.InitialVelocity*c.InitialVelocity+2*c.Gravity*(c.GroundY-c.StartY)))/c.Gravity
 p:=DropPose{X:c.X,ScaleX:1,ScaleY:1,Reveal:math.Min(1,t/c.RevealTime.Seconds())}
 compression:=0.0
 if t<hit {
  p.BottomY=c.StartY+c.InitialVelocity*t+0.5*c.Gravity*t*t
 } else {
  remaining:=t-hit
  velocity:=c.InitialVelocity+c.Gravity*hit
  amplitude:=0.10
  for b:=0;b<1000;b++ {
   if remaining<c.SquashTime.Seconds() {compression+=amplitude*math.Sin(math.Pi*remaining/c.SquashTime.Seconds())}
   velocity*=c.Restitution
   if velocity<c.SettleSpeed {
    p.BottomY=c.GroundY;p.Settled=true;break
   }
   flight:=2*velocity/c.Gravity
   if remaining<flight {
    p.BottomY=c.GroundY-velocity*remaining+0.5*c.Gravity*remaining*remaining
    break
   }
   remaining-=flight
   amplitude*=c.Restitution
   if b==999 { p.BottomY=c.GroundY;p.Settled=true }
  }
 }
 p.BottomY=math.Min(c.GroundY,math.Max(c.StartY,p.BottomY))
 compression=math.Min(.12,compression)
 p.ScaleY=1-compression;p.ScaleX=1+compression
 height:=c.GroundY-p.BottomY
 p.ShadowOpacity=0.15+0.20*(1-math.Min(1,height/(c.GroundY-c.StartY)))
 p.ShadowScale=0.60+0.40*(1-math.Min(1,height/(c.GroundY-c.StartY)))
 return p,nil
}
