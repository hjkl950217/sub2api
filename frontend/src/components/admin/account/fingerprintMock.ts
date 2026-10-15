// FORK: lm-detector 模型指纹检测——类型定义、示例数据与 mock 调用
//
// 结构取自一次真实检测（sub2api 账号 3285「云枢」/ https://yunshu.best /
// 请求模型 claude-opus-5-5 / 协议 message），完整原始返回见
// 临时文件/lm-detector-sample-output.json。
//
// 接后端时只需要把 runFingerprintMock 换成真实请求，类型和 UI 都不用动。
//
// 两个和直觉相反的实测事实：
// 1. evidence.insufficient 在成功检测时也是 true —— 它表达「证据不足以证明身份」，
//    不是「检测失败」，UI 不能把它渲染成错误态。
// 2. results 是参考库全部模型的完整排行（本次 59 条），不是 top-N，UI 要自己截断。

export interface FingerprintCandidate {
  model: string
  display_name: string
  family: string
  family_name: string
  score: number
  probability: number
}

export interface FingerprintDiagnostic {
  index: number
  parsed_numbers: number
  minimum_numbers: number
  accepted: boolean
  raw_out_of_range_count: number
}

export interface FingerprintEvidence {
  insufficient: boolean
  label: string
  reason: string
  threshold: number | null
  method: string
}

export interface FingerprintAnalysis {
  prediction: string
  prediction_name: string
  family_prediction: string
  family_prediction_name: string
  probability: number
  probability_status: string
  probability_scope: string
  ranking_score: number
  used_outputs: number
  method: string
  model_version: Record<string, unknown>
  calibration: Record<string, unknown>
  results: FingerprintCandidate[]
  diagnostics: FingerprintDiagnostic[]
  evidence: FingerprintEvidence
}

export interface FingerprintReport {
  schema: string
  created_at: string
  request: Record<string, unknown>
  bank: { reference_sha256: string; models: number }
  requested_rounds: number
  completed_rounds: number
  scored_rounds: number
  analysis: FingerprintAnalysis
}

export const FINGERPRINT_SAMPLE: FingerprintReport = {
  "schema": "fpd-detection-v1",
  "created_at": "2026-10-15T03:10:53.204Z",
  "request": {
    "model": "claude-opus-5-5",
    "baseUrl": "https://yunshu.best",
    "format": "anthropic",
    "serviceTier": "default",
    "stream": true,
    "effort": "",
    "api": "message",
    "count": 3,
    "parallel": 3,
    "repeat": 1,
    "strict": false,
    "timeout_seconds": 120,
    "tokenizer": false
  },
  "bank": {
    "reference_sha256": "44d2447c613889108dde43c4fa9784a7a4ebcfbf5d67d5d2f7124ad3d7fb068a",
    "models": 59
  },
  "requested_rounds": 1,
  "completed_rounds": 1,
  "scored_rounds": 1,
  "analysis": {
    "probability": 0.46715637473108274,
    "probability_status": "reference_calibrated",
    "used_outputs": 3,
    "diagnostics": [
      {
        "index": 0,
        "parsed_numbers": 284,
        "minimum_numbers": 162,
        "accepted": true,
        "raw_out_of_range_count": 0
      },
      {
        "index": 1,
        "parsed_numbers": 278,
        "minimum_numbers": 162,
        "accepted": true,
        "raw_out_of_range_count": 0
      },
      {
        "index": 2,
        "parsed_numbers": 331,
        "minimum_numbers": 183,
        "accepted": true,
        "raw_out_of_range_count": 0
      }
    ],
    "method": "shared-detector-v2",
    "model_version": {
      "base_sha256": "3aba0410753abbbb92e90c2ad995af312a10dbecc6c4bc7abed35d95df8a0b70"
    },
    "prediction": "mimo-v2.5",
    "prediction_name": "mimo-v2.5",
    "family_prediction": "mimo",
    "family_prediction_name": "MiMo",
    "results": [
      {
        "model": "mimo-v2.5",
        "display_name": "mimo-v2.5",
        "family": "mimo",
        "family_name": "MiMo",
        "score": 1.9041367174929367,
        "probability": 0.46715637473108274
      },
      {
        "model": "gemini-4-argon",
        "display_name": "gemini-4-argon",
        "family": "gemini",
        "family_name": "Gemini",
        "score": 1.7484951160669542,
        "probability": 0.22190148696682
      },
      {
        "model": "mimo-v2.6-flash",
        "display_name": "mimo-v2.6-flash",
        "family": "mimo",
        "family_name": "MiMo",
        "score": 1.629987571814792,
        "probability": 0.12589074512463147
      },
      {
        "model": "grok-4.5",
        "display_name": "grok-4.5",
        "family": "grok",
        "family_name": "Grok",
        "score": 1.5591272440524473,
        "probability": 0.08970181158366272
      },
      {
        "model": "hy4-preview",
        "display_name": "hy4-preview",
        "family": "hunyuan",
        "family_name": "Hunyuan",
        "score": 1.2702008739778428,
        "probability": 0.02252356709598488
      },
      {
        "model": "qwen3.8-omni-flash",
        "display_name": "qwen3.8-omni-flash",
        "family": "qwen",
        "family_name": "Qwen",
        "score": 1.1279067354157883,
        "probability": 0.01140409273391435
      },
      {
        "model": "mimo-v2.5-pro",
        "display_name": "mimo-v2.5-pro",
        "family": "mimo",
        "family_name": "MiMo",
        "score": 1.1152802173569807,
        "probability": 0.010735756843503187
      },
      {
        "model": "seed-2.0-lite",
        "display_name": "seed-2.0-lite",
        "family": "seed",
        "family_name": "Seed",
        "score": 1.0854303367765503,
        "probability": 0.009307388783526688
      },
      {
        "model": "qwen3.8-flash",
        "display_name": "qwen3.8-flash",
        "family": "qwen",
        "family_name": "Qwen",
        "score": 1.068897808183617,
        "probability": 0.008599756075421247
      },
      {
        "model": "qwen3.8-27b",
        "display_name": "qwen3.8-27b",
        "family": "qwen",
        "family_name": "Qwen",
        "score": 1.0269312731302414,
        "probability": 0.007035781566292481
      },
      {
        "model": "seed-2.1-turbo",
        "display_name": "seed-2.1-turbo",
        "family": "seed",
        "family_name": "Seed",
        "score": 1.0234167392084432,
        "probability": 0.006918499003494605
      },
      {
        "model": "deepseek-v3.2",
        "display_name": "deepseek-v3.2",
        "family": "deepseek",
        "family_name": "DeepSeek",
        "score": 1.0184902397251914,
        "probability": 0.006757381718204644
      },
      {
        "model": "gemini-3.8-flash",
        "display_name": "gemini-3.8-flash",
        "family": "gemini",
        "family_name": "Gemini",
        "score": 0.8739827550309224,
        "probability": 0.0033853565129595533
      },
      {
        "model": "grok-4.6",
        "display_name": "grok-4.6",
        "family": "grok",
        "family_name": "Grok",
        "score": 0.8074781245879634,
        "probability": 0.002462972533812169
      },
      {
        "model": "kimi-k3",
        "display_name": "kimi-k3",
        "family": "kimi",
        "family_name": "Kimi",
        "score": 0.6648272868466724,
        "probability": 0.001244922150250312
      },
      {
        "model": "gemini-3.1-pro-preview",
        "display_name": "gemini-3.1-pro-preview",
        "family": "gemini",
        "family_name": "Gemini",
        "score": 0.5741755156542426,
        "probability": 0.0008069347047002013
      },
      {
        "model": "claude-haiku-4.5",
        "display_name": "claude-haiku-4.5",
        "family": "claude",
        "family_name": "Claude",
        "score": 0.5458255940583817,
        "probability": 0.000704610855608838
      },
      {
        "model": "gemini-3.6-flash",
        "display_name": "gemini-3.6-flash",
        "family": "gemini",
        "family_name": "Gemini",
        "score": 0.5038205941739139,
        "probability": 0.0005763624047605012
      },
      {
        "model": "muse-spark-1.2",
        "display_name": "muse-spark-1.2",
        "family": "muse",
        "family_name": "Muse",
        "score": 0.4720724130889925,
        "probability": 0.0004951623816553969
      },
      {
        "model": "claude-sonnet-4.6",
        "display_name": "claude-sonnet-4.6",
        "family": "claude",
        "family_name": "Claude",
        "score": 0.453220813431107,
        "probability": 0.0004524688613833637
      },
      {
        "model": "gemini-3.7-flash",
        "display_name": "gemini-3.7-flash",
        "family": "gemini",
        "family_name": "Gemini",
        "score": 0.3927371573183508,
        "probability": 0.0003388057335053249
      },
      {
        "model": "glm-5.3-flashx",
        "display_name": "glm-5.3-flashx",
        "family": "glm",
        "family_name": "GLM",
        "score": 0.3390370033261568,
        "probability": 0.0002620617527899038
      },
      {
        "model": "glm-5.3-flash",
        "display_name": "glm-5.3-flash",
        "family": "glm",
        "family_name": "GLM",
        "score": 0.32945736404137366,
        "probability": 0.00025032522178414235
      },
      {
        "model": "muse-wonder",
        "display_name": "muse-wonder",
        "family": "muse",
        "family_name": "Muse",
        "score": 0.31299870336885227,
        "probability": 0.00023137494432329847
      },
      {
        "model": "deepseek-v4.1-flash",
        "display_name": "deepseek-v4.1-flash",
        "family": "deepseek",
        "family_name": "DeepSeek",
        "score": 0.2936862990740635,
        "probability": 0.00021096003436829933
      },
      {
        "model": "step-5-preview",
        "display_name": "step-5-preview",
        "family": "step",
        "family_name": "Step",
        "score": 0.27119858117398576,
        "probability": 0.0001894472120206878
      },
      {
        "model": "gpt-6-astra",
        "display_name": "gpt-6-astra/6.1-sol",
        "family": "gpt",
        "family_name": "GPT",
        "score": 0.13360757085307418,
        "probability": 9.810275995305829e-05
      },
      {
        "model": "gpt-6-sol",
        "display_name": "gpt-6-sol",
        "family": "gpt",
        "family_name": "GPT",
        "score": 0.04637446705624801,
        "probability": 6.46366373479011e-05
      },
      {
        "model": "glm-5.3",
        "display_name": "glm-5.3",
        "family": "glm",
        "family_name": "GLM",
        "score": 0.03859316378527549,
        "probability": 6.227521920190933e-05
      },
      {
        "model": "qwen3.8-max-0902",
        "display_name": "qwen3.8-max-0902",
        "family": "qwen",
        "family_name": "Qwen",
        "score": -0.04994860833036194,
        "probability": 4.077504039800496e-05
      },
      {
        "model": "claude-opus-4.8",
        "display_name": "claude-opus-4.8",
        "family": "claude",
        "family_name": "Claude",
        "score": -0.11263393527628532,
        "probability": 3.0212251872033766e-05
      },
      {
        "model": "glm-5.2",
        "display_name": "glm-5.2",
        "family": "glm",
        "family_name": "GLM",
        "score": -0.13915692735351992,
        "probability": 2.6612701919331668e-05
      },
      {
        "model": "claude-fable-5.1",
        "display_name": "claude-fable-5.1",
        "family": "claude",
        "family_name": "Claude",
        "score": -0.14965967461964796,
        "probability": 2.5308851529016657e-05
      },
      {
        "model": "deepseek-v4-pro-0813",
        "display_name": "deepseek-v4-pro-0813",
        "family": "deepseek",
        "family_name": "DeepSeek",
        "score": -0.16680400137019402,
        "probability": 2.331631293450219e-05
      },
      {
        "model": "muse-spark-1.3",
        "display_name": "muse-spark-1.3",
        "family": "muse",
        "family_name": "Muse",
        "score": -0.19336527408290666,
        "probability": 2.0534599222234634e-05
      },
      {
        "model": "deepseek-v4-pro",
        "display_name": "deepseek-v4-pro",
        "family": "deepseek",
        "family_name": "DeepSeek",
        "score": -0.28966359382156626,
        "probability": 1.2955473769957141e-05
      },
      {
        "model": "claude-opus-4.6",
        "display_name": "claude-opus-4.6",
        "family": "claude",
        "family_name": "Claude",
        "score": -0.32640866570263094,
        "probability": 1.0867399917384369e-05
      },
      {
        "model": "kimi-k2.8-preview",
        "display_name": "kimi-k2.8-preview",
        "family": "kimi",
        "family_name": "Kimi",
        "score": -0.44507331831174846,
        "probability": 6.160740159799871e-06
      },
      {
        "model": "gpt-4o",
        "display_name": "gpt-4o",
        "family": "gpt",
        "family_name": "GPT",
        "score": -0.48838510433589144,
        "probability": 5.008004641541864e-06
      },
      {
        "model": "claude-opus-5",
        "display_name": "claude-opus-5",
        "family": "claude",
        "family_name": "Claude",
        "score": -0.5015798806540318,
        "probability": 4.7017144642076105e-06
      },
      {
        "model": "gpt-5.4",
        "display_name": "gpt-5.4",
        "family": "gpt",
        "family_name": "GPT",
        "score": -0.5199822002475577,
        "probability": 4.305569049712917e-06
      },
      {
        "model": "mimo-v2.6-pro",
        "display_name": "mimo-v2.6-pro",
        "family": "mimo",
        "family_name": "MiMo",
        "score": -0.5487091176755806,
        "probability": 3.752825669596765e-06
      },
      {
        "model": "deepseek-v4-flash-0731",
        "display_name": "deepseek-v4-flash-0731",
        "family": "deepseek",
        "family_name": "DeepSeek",
        "score": -0.5520307653637377,
        "probability": 3.693674259046163e-06
      },
      {
        "model": "grok-4.7",
        "display_name": "grok-4.7",
        "family": "grok",
        "family_name": "Grok",
        "score": -0.608207567443802,
        "probability": 2.823364415324548e-06
      },
      {
        "model": "gpt-5.6-sol",
        "display_name": "gpt-5.6-sol",
        "family": "gpt",
        "family_name": "GPT",
        "score": -0.6916114559634348,
        "probability": 1.89460462719518e-06
      },
      {
        "model": "deepseek-v4-flash",
        "display_name": "deepseek-v4-flash",
        "family": "deepseek",
        "family_name": "DeepSeek",
        "score": -0.6931558440950092,
        "probability": 1.8806611690367878e-06
      },
      {
        "model": "gemini-3.5-flash",
        "display_name": "gemini-3.5-flash",
        "family": "gemini",
        "family_name": "Gemini",
        "score": -0.7608459392165062,
        "probability": 1.3605147125734728e-06
      },
      {
        "model": "gpt-5.6-terra",
        "display_name": "gpt-5.6-terra",
        "family": "gpt",
        "family_name": "GPT",
        "score": -0.7832381990777972,
        "probability": 1.2223330679375519e-06
      },
      {
        "model": "claude-fable-5",
        "display_name": "claude-fable-5",
        "family": "claude",
        "family_name": "Claude",
        "score": -0.8766075211669135,
        "probability": 7.820613186547174e-07
      },
      {
        "model": "gemini-2.5-pro",
        "display_name": "gemini-2.5-pro",
        "family": "gemini",
        "family_name": "Gemini",
        "score": -0.8805552016723783,
        "probability": 7.674332284296466e-07
      },
      {
        "model": "gpt-5.5",
        "display_name": "gpt-5.5",
        "family": "gpt",
        "family_name": "GPT",
        "score": -0.9345160339722183,
        "probability": 5.928597999768197e-07
      },
      {
        "model": "claude-opus-4.7",
        "display_name": "claude-opus-4.7",
        "family": "claude",
        "family_name": "Claude",
        "score": -0.9735297270934793,
        "probability": 4.919397594607546e-07
      },
      {
        "model": "claude-opus-5.5",
        "display_name": "claude-opus-5.5",
        "family": "claude",
        "family_name": "Claude",
        "score": -1.0413026215484937,
        "probability": 3.5573993136095695e-07
      },
      {
        "model": "seed-2.0-mini",
        "display_name": "seed-2.0-mini",
        "family": "seed",
        "family_name": "Seed",
        "score": -1.3678270550102627,
        "probability": 7.4622375867623e-08
      },
      {
        "model": "claude-sonnet-5",
        "display_name": "claude-sonnet-5",
        "family": "claude",
        "family_name": "Claude",
        "score": -1.404122390677458,
        "probability": 6.273004857406615e-08
      },
      {
        "model": "claude-sonnet-5.5",
        "display_name": "claude-sonnet-5.5",
        "family": "claude",
        "family_name": "Claude",
        "score": -1.4761891450121842,
        "probability": 4.44403037446418e-08
      },
      {
        "model": "claude-haiku-5.5",
        "display_name": "claude-haiku-5.5",
        "family": "claude",
        "family_name": "Claude",
        "score": -1.8344875356875812,
        "probability": 8.007776248181389e-09
      },
      {
        "model": "gpt-5.6-luna",
        "display_name": "gpt-5.6-luna",
        "family": "gpt",
        "family_name": "GPT",
        "score": -1.8650732904166978,
        "probability": 6.91796794019548e-09
      },
      {
        "model": "gpt-6-luna",
        "display_name": "gpt-6-luna",
        "family": "gpt",
        "family_name": "GPT",
        "score": -1.956723684871485,
        "probability": 4.462727646002692e-09
      }
    ],
    "ranking_score": 1.9041367174929367,
    "probability_scope": "reference-closed-set",
    "calibration": {
      "method": "ranking-temperature",
      "run": ".training/20261013T162144279078Z/calibration",
      "tau": 4.782979057955927,
      "sha256": "3506548c7e094bcebae4ec9ef5dd9c011434629365010dde729658a81c0ac648"
    },
    "evidence": {
      "insufficient": true,
      "label": "Calibrated ranking",
      "reason": "Confidence is a closed-set probability within the reference bank.",
      "threshold": null,
      "method": "reference-calibrated-ranking"
    }
  }
}

/** mock 延迟：3 秒，用来预览「检测中」和「检测完成」两种样式 */
export const FINGERPRINT_MOCK_DELAY_MS = 3000

/** mock 调用：延迟 3 秒后返回示例报告。接后端时把这个函数体换成真实请求即可。 */
export function runFingerprintMock(): Promise<FingerprintReport> {
  return new Promise((resolve) => {
    setTimeout(() => resolve(JSON.parse(JSON.stringify(FINGERPRINT_SAMPLE)) as FingerprintReport), FINGERPRINT_MOCK_DELAY_MS)
  })
}
