{
  description = "rules_img";

  inputs = {
    bazel-env.url = "github:malt3/bazel-env";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { nixpkgs, flake-utils, bazel-env, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
        bazel_pkgs = bazel-env.packages.${system};
      in
      rec {
        packages.dev = (bazel_pkgs.bazel-env.override {
          name = "dev";
          extraPkgs = [
            pkgs.pre-commit
          ];
        });
        devShells.dev = packages.dev.env;
        devShells.default = pkgs.mkShell {
          packages = [ packages.dev pkgs.pre-commit ];
        };
      });
}
