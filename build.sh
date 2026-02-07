if [ ! -d "bin" ]; then
    mkdir bin
else
    echo "Cleaning existing bin directory..."
    rm -rf bin/*
fi
echo "Building GateSentry..."
go build -o bin/ ./...
if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi
echo "Build successful. Executable is in the 'bin' directory."
