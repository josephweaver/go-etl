#!/usr/bin/env Rscript

suppressPackageStartupMessages({
  library(brms)
  library(cmdstanr)
  library(dplyr)
  library(jsonlite)
  library(posterior)
  library(readr)
  library(tibble)
  library(tidyr)
})

args <- commandArgs(trailingOnly = TRUE)
if (length(args) < 1L) {
  stop("usage: fixture.R <control|prepare|sample|compare> [backend]")
}

mode <- args[[1L]]
backend <- if (length(args) >= 2L) args[[2L]] else ""
root <- Sys.getenv("GOETL_BRMS_TEST_ROOT", "/checkpoint-test")
run_name <- Sys.getenv("GOETL_BRMS_RUN", "run")
iterations <- as.integer(Sys.getenv("GOETL_BRMS_ITER", "4000"))
warmup <- as.integer(Sys.getenv(
  "GOETL_BRMS_WARMUP",
  as.character(iterations %/% 2L)
))
seed <- as.integer(Sys.getenv("GOETL_BRMS_SEED", "8102026"))

dir.create(root, recursive = TRUE, showWarnings = FALSE, mode = "0700")
state_dir <- file.path(root, "state")
output_dir <- file.path(root, "output")
marker_dir <- file.path(root, "markers")
dir.create(state_dir, recursive = TRUE, showWarnings = FALSE, mode = "0700")
dir.create(output_dir, recursive = TRUE, showWarnings = FALSE, mode = "0700")
dir.create(marker_dir, recursive = TRUE, showWarnings = FALSE, mode = "0700")

write_marker <- function(name, value = format(Sys.time(), tz = "UTC", usetz = TRUE)) {
  path <- file.path(marker_dir, paste(run_name, name, sep = "."))
  if (file.exists(path)) {
    stop("marker already exists: ", path)
  }
  writeLines(value, path, useBytes = TRUE)
  close(file(path, open = "a"))
  path
}

make_data <- function() {
  crossing(
    group = factor(sprintf("g%02d", seq_len(20L))),
    observation = seq_len(50L)
  ) |>
    arrange(group, observation) |>
    group_by(group) |>
    mutate(
      x = (observation - mean(observation)) / 10,
      group_effect = (as.integer(group) - 10.5) / 8,
      deterministic_error = ((observation %% 7L) - 3L) / 9,
      y = 1.25 + 0.7 * x + group_effect + deterministic_error
    ) |>
    ungroup() |>
    select(group, observation, x, y)
}

model_formula <- bf(y ~ x + (1 | group))
model_prior <- c(
  prior(normal(0, 2), class = "b"),
  prior(normal(0, 2), class = "Intercept"),
  prior(exponential(1), class = "sd"),
  prior(exponential(1), class = "sigma")
)

configure_backend <- function(selected_backend) {
  if (!selected_backend %in% c("rstan", "cmdstanr")) {
    stop("unsupported backend: ", selected_backend)
  }
  options(mc.cores = 1L)
  rstan::rstan_options(auto_write = TRUE)
  if (selected_backend == "cmdstanr") {
    cmdstanr::set_cmdstan_path(Sys.getenv("CMDSTAN"))
    cmdstan_model_dir <- file.path(state_dir, "cmdstan-model")
    dir.create(
      cmdstan_model_dir,
      recursive = TRUE,
      showWarnings = FALSE,
      mode = "0700"
    )
    options(cmdstanr_write_stan_file_dir = cmdstan_model_dir)
  }
}

prepared_path <- function(selected_backend) {
  file.path(state_dir, paste0("prepared-", selected_backend, ".rds"))
}

fit_path <- function(selected_backend, selected_run) {
  file.path(output_dir, paste(selected_run, selected_backend, "fit.rds", sep = "."))
}

draw_path <- function(selected_backend, selected_run) {
  file.path(output_dir, paste(selected_run, selected_backend, "draws.csv", sep = "."))
}

summary_path <- function(selected_backend, selected_run) {
  file.path(output_dir, paste(selected_run, selected_backend, "summary.json", sep = "."))
}

write_fit_outputs <- function(fit, selected_backend, selected_run) {
  saveRDS(fit, fit_path(selected_backend, selected_run), version = 3L)
  draws <- as_draws_matrix(fit)
  write_csv(as.data.frame(draws), draw_path(selected_backend, selected_run))
  fixed <- fixef(fit)
  payload <- list(
    schema = "goetl/dmtcp-r-brms-fixture-result/v1",
    backend = selected_backend,
    formula = paste(deparse(model_formula), collapse = " "),
    iterations = iterations,
    warmup = warmup,
    chains = 1L,
    draws = nrow(draws),
    parameters = ncol(draws),
    finite_fixed_effects = all(is.finite(fixed)),
    completed = TRUE
  )
  write_json(payload, summary_path(selected_backend, selected_run),
    auto_unbox = TRUE, pretty = TRUE
  )
}

if (mode == "control") {
  data <- make_data() |>
    group_by(group) |>
    summarise(mean_y = mean(y), mean_x = mean(x), .groups = "drop")
  write_csv(data, file.path(output_dir, paste0(run_name, ".control.csv")))
  write_marker("control-ready")
  release_path <- file.path(root, "control.release")
  while (!file.exists(release_path)) {
    Sys.sleep(0.2)
  }
  write_marker("control-complete")
  quit(status = 0L)
}

configure_backend(backend)

if (mode == "prepare") {
  data <- make_data()
  write_csv(data, file.path(state_dir, "model-data.csv"))
  fit <- brm(
    formula = model_formula,
    data = data,
    family = gaussian(),
    prior = model_prior,
    backend = backend,
    chains = 1L,
    cores = 1L,
    iter = 200L,
    warmup = 100L,
    seed = seed,
    refresh = 0L,
    silent = 2L
  )
  saveRDS(fit, prepared_path(backend), version = 3L)
  write_marker(paste0(backend, "-prepared"))
  quit(status = 0L)
}

if (mode == "sample") {
  prepared <- readRDS(prepared_path(backend))
  data <- read_csv(file.path(state_dir, "model-data.csv"), show_col_types = FALSE)
  data$group <- factor(data$group)
  if (backend == "cmdstanr") {
    model <- brms:::compile_model(
      stancode(prepared),
      backend = backend,
      threads = prepared$threads,
      opencl = prepared$opencl,
      silent = 0L
    )
    attr(prepared$fit, "CmdStanModel") <- model
  }
  write_marker(paste0(backend, "-sampling-called"))
  fit <- update(
    prepared,
    newdata = data,
    backend = backend,
    chains = 1L,
    cores = 1L,
    iter = iterations,
    warmup = warmup,
    seed = seed,
    refresh = 50L,
    silent = 0L,
    recompile = FALSE
  )
  write_fit_outputs(fit, backend, run_name)
  write_marker(paste0(backend, "-complete"))
  quit(status = 0L)
}

if (mode == "compare") {
  if (length(args) != 4L) {
    stop("usage: fixture.R compare <backend> <expected-run> <actual-run>")
  }
  expected_run <- args[[3L]]
  actual_run <- args[[4L]]
  expected <- as_draws_matrix(readRDS(fit_path(backend, expected_run)))
  actual <- as_draws_matrix(readRDS(fit_path(backend, actual_run)))
  if (!identical(dim(expected), dim(actual))) {
    stop("posterior draw dimensions differ")
  }
  if (!identical(colnames(expected), colnames(actual))) {
    stop("posterior draw columns differ")
  }
  difference <- max(abs(expected - actual))
  if (!is.finite(difference) || difference != 0) {
    stop("posterior draws differ; max absolute difference: ", difference)
  }
  write_json(
    list(
      schema = "goetl/dmtcp-r-brms-compare/v1",
      backend = backend,
      expected_run = expected_run,
      actual_run = actual_run,
      draws = nrow(actual),
      parameters = ncol(actual),
      max_absolute_difference = difference,
      match = TRUE
    ),
    file.path(output_dir, paste0("compare.", backend, ".json")),
    auto_unbox = TRUE,
    pretty = TRUE
  )
  quit(status = 0L)
}

stop("unsupported mode: ", mode)
