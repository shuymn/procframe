BIN := greeter

.PHONY: build run clean

build:
	go build -o $(BIN) .

run: build
	./$(BIN) greet run --name World

clean:
	rm -f $(BIN)
