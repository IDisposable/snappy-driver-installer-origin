cd libwebp-1.6.0
./configure --prefix=$PREFIX
make install CFLAGS='-O2 -mstackrealign'
