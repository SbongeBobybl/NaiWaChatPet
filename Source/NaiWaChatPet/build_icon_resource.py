"""Embed the original frog icon as a native AMD64 COFF .rsrc object for Go."""
from pathlib import Path
import struct

root=Path(__file__).resolve().parent
ico=(root/'icons/NaiWaChatPet.ico').read_bytes()
count=struct.unpack_from('<H',ico,4)[0]
resources=[];group=struct.pack('<HHH',0,1,count)
for i in range(count):
    w,h,colors,reserved,planes,bits,size,offset=struct.unpack_from('<BBBBHHII',ico,6+16*i)
    resources.append((3,i+1,ico[offset:offset+size]))
    group+=struct.pack('<BBBBHHIH',w,h,colors,reserved,planes,bits,size,i+1)
resources.append((14,1,group))

tree={}
for kind,ident,payload in resources: tree.setdefault(kind,{})[ident]={0x409:payload}
data=bytearray();leaves=[]
def directory(items):
    position=len(data)
    data.extend(struct.pack('<IIHHHH',0,0,0,0,0,len(items)))
    offsets=[]
    for ident,value in sorted(items.items()):
        offsets.append((len(data),value));data.extend(struct.pack('<II',ident,0))
    for entry,value in offsets:
        if isinstance(value,dict):
            target=directory(value);struct.pack_into('<I',data,entry+4,target|0x80000000)
        else:
            target=len(data);data.extend(b'\0'*16);leaves.append((target,value));struct.pack_into('<I',data,entry+4,target)
    return position
directory(tree)
relocations=[]
for entry,payload in leaves:
    while len(data)%4:data.append(0)
    position=len(data);data.extend(payload)
    struct.pack_into('<IIII',data,entry,position,len(payload),0,0)
    relocations.append(struct.pack('<IIH',entry,0,0x0003)) # IMAGE_REL_AMD64_ADDR32NB
header_size=20+40
reloc_offset=header_size+len(data)
symbol_offset=reloc_offset+10*len(relocations)
header=struct.pack('<HHIIIHH',0x8664,1,0,symbol_offset,1,0,0)
section=struct.pack('<8sIIIIIIHHI',b'.rsrc\0\0\0',0,0,len(data),header_size,reloc_offset,0,len(relocations),0,0x40000040)
symbol=struct.pack('<8sIhHBB',b'.rsrc\0\0\0',0,1,0,3,0)
out=root/'source/cmd/codexpet/app_windows_amd64.syso'
out.write_bytes(header+section+data+b''.join(relocations)+symbol+struct.pack('<I',4))
print(f'Embedded {count} original frog icon resolutions into {out.name}')
