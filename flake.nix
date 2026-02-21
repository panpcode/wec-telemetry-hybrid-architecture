{
  description = "WEC Telemetry Hybrid Architecture - Development Environment";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go
            go
            
            # Docker & Docker Compose
            docker
            docker-compose
            
            # Database tools
            postgresql
            
            # Development utilities
            git
            curl
            jq
            
            # Node.js for frontend
            nodejs
            yarn
            
            # Build tools
            gnumake
            pkg-config
          ];

          shellHook = ''
            echo "🏎️  WEC Telemetry Development Environment"
            echo "Go: $(go version)"
            echo "Node: $(node --version)"
            echo "Docker: $(docker --version)"
            echo ""
            echo "Available commands:"
            echo "  make build      - Build all services"
            echo "  make test       - Run tests"
            echo "  make dev        - Start development services"
          '';
        };
      }
    );
}
