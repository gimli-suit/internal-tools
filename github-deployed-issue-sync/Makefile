BINARY = github-deployed-issue-sync
INSTALL_DIR = /usr/local/bin
CONF_DIR = /etc/github-deployed-issue-sync
ENV_FILE = $(CONF_DIR)/.env
LOG_DIR = /var/log/github-deployed-issue-sync
LOG_FILE = $(LOG_DIR)/github-deployed-issue-sync.log
CRON_JOB = */30 * * * * cd $(CONF_DIR) && $(INSTALL_DIR)/$(BINARY) >> $(LOG_FILE) 2>&1

.PHONY: build run dry-run check-auth install test clean install-cron uninstall-cron verify-cron

clean:
	rm -f $(BINARY)

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

DRY_RUN_LOG = dry-run.log

dry-run: build
	./$(BINARY) -dry-run 2>&1 | tee $(DRY_RUN_LOG)
	@echo "Log saved to $(DRY_RUN_LOG)"

check-auth: build
	./$(BINARY) -check-auth

test:
	go test ./...

install: build
	sudo mkdir -p $(INSTALL_DIR)
	sudo cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	sudo mkdir -p $(LOG_DIR)
	sudo touch $(LOG_FILE)
	sudo chown $(shell id -u):$(shell id -g) $(LOG_FILE)
	sudo mkdir -p $(CONF_DIR)
	@if [ -f .env ]; then \
		sudo cp .env $(ENV_FILE); \
	elif [ ! -f $(ENV_FILE) ]; then \
		echo "WARNING: No .env file found. Create $(ENV_FILE) with your tokens before running."; \
		sudo cp .env.example $(ENV_FILE); \
	fi
	@if [ -f config.json ]; then \
		sudo cp config.json $(CONF_DIR)/config.json; \
	elif [ ! -f $(CONF_DIR)/config.json ]; then \
		echo "WARNING: No config.json found. Create $(CONF_DIR)/config.json before running."; \
	fi
	sudo chown -R $(shell id -u):$(shell id -g) $(CONF_DIR)
	sudo chmod 700 $(CONF_DIR)
	sudo chmod 600 $(ENV_FILE) 2>/dev/null || true

install-cron: install
	(crontab -l 2>/dev/null | grep -v '$(BINARY)'; echo '$(CRON_JOB)') | crontab -
	@echo "Cron job installed: $(CRON_JOB)"
	@echo "Logs: $(LOG_FILE)"

verify-cron:
	@echo "Checking for cron job..."
	@crontab -l 2>/dev/null | grep '$(BINARY)' && echo "Cron job is installed." || (echo "ERROR: Cron job not found." && exit 1)
	@echo "Checking for binary..."
	@test -x $(INSTALL_DIR)/$(BINARY) && echo "Binary installed at $(INSTALL_DIR)/$(BINARY)." || (echo "ERROR: Binary not found at $(INSTALL_DIR)/$(BINARY)." && exit 1)
	@echo "Checking for log file..."
	@test -f $(LOG_FILE) && echo "Log file exists at $(LOG_FILE)." || (echo "ERROR: Log file not found at $(LOG_FILE)." && exit 1)
	@echo "All checks passed."

uninstall-cron:
	crontab -l 2>/dev/null | grep -v '$(BINARY)' | crontab -
	@echo "Cron job removed"
