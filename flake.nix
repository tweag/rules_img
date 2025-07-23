{
  description = "rules_img";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    bazel-env.url = "github:malt3/bazel-env";
    bazel-env.inputs.nixpkgs.follows = "nixpkgs";
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
        packages.dev = (bazel_pkgs.bazel-full-env.override {
          name = "dev";
          extraPkgs = [
            pkgs.pre-commit
          ];
        });
        packages.bazel-fhs = bazel_pkgs.bazel-full;
        devShells.dev = packages.dev.env;
        devShells.default = pkgs.mkShell {
          packages = [ packages.dev bazel_pkgs.bazel-full pkgs.pre-commit pkgs.tweag-credential-helper ];
        };
      });
}
